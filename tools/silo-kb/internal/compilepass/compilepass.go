// Package compilepass runs the knowledge lifecycle over knowledge/**:
// reinforce → decay → archive → graduate, then appends an audit entry to
// knowledge/log.md. Reinforcement and graduation are explicit CLI inputs —
// the agent driving a run decides and justifies them; nothing is inferred.
// projects/** is asserted canon and is never touched.
package compilepass

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"silo.local/silo-kb/internal/validate"
	"silo.local/silo-kb/internal/vault"
)

const (
	reinforceDelta   = 0.1
	decayDelta       = 0.1
	staleAfter       = 30 * 24 * time.Hour
	ancientAfter     = 6 * 30 * 24 * time.Hour
	stableMinRuns    = 3
	developingAt     = 0.8
	stableAt         = 0.9
)

type Options struct {
	Reinforce []string          // note ids or vault-relative paths
	Graduate  map[string]string // id (or path) -> vault-relative destination under projects/
	DryRun    bool
	Now       time.Time
}

type Action struct {
	Kind    string // reinforced | decayed | archived-faded | archived-ancient | graduated | promoted
	Note    string // wikilink basename
	Detail  string
}

type Report struct {
	When    time.Time
	Actions []Action
}

func (r *Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s compile\n", r.When.Format(validate.TimeLayout))
	if len(r.Actions) == 0 {
		b.WriteString("- no changes\n")
		return b.String()
	}
	for _, a := range r.Actions {
		fmt.Fprintf(&b, "- %s: [[%s]] %s\n", a.Kind, a.Note, a.Detail)
	}
	return b.String()
}

// Run executes a compilation pass rooted at repoRoot (the directory containing
// knowledge-base/).
func Run(repoRoot string, opts Options) (*Report, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	vaultRoot := filepath.Join(repoRoot, "knowledge-base")
	notes, err := vault.Walk(vaultRoot)
	if err != nil {
		return nil, err
	}

	reinforce := toSet(opts.Reinforce)
	graduate := map[string]string{}
	for k, v := range opts.Graduate {
		graduate[k] = v
	}

	report := &Report{When: opts.Now}

	for _, n := range notes {
		if n.Tier != vault.TierKnowledge || strings.HasPrefix(n.Path, "knowledge/archive/") {
			continue
		}
		name := wikiname(n.Path)
		abs := filepath.Join(vaultRoot, n.Path)

		conf := toFloat(n.Frontmatter["confidence"])
		count := toInt(n.Frontmatter["reinforce_count"])
		maturity, _ := n.Frontmatter["maturity"].(string)
		last, err := validate.TimeOf(n.Frontmatter["last_reinforced"])
		if err != nil {
			return nil, fmt.Errorf("%s: last_reinforced: %v", n.Path, err)
		}
		lastStr := last.Format(validate.TimeLayout)

		reinforced := reinforce[n.ID()] || reinforce[n.Path]
		changed := false

		// 1. Reinforce (wins over decay in the same run).
		if reinforced {
			old := conf
			conf = min(1.0, conf+reinforceDelta)
			count++
			last = opts.Now
			changed = true
			report.Actions = append(report.Actions, Action{"reinforced", name,
				fmt.Sprintf("%.1f→%.1f", old, conf)})
			// Maturity promotions only advance on reinforcement.
			if maturity == "seed" && conf >= developingAt {
				maturity = "developing"
				report.Actions = append(report.Actions, Action{"promoted", name, "seed→developing"})
			} else if maturity == "developing" && conf >= stableAt && count >= stableMinRuns {
				maturity = "stable"
				report.Actions = append(report.Actions, Action{"promoted", name, "developing→stable"})
			}
		} else if opts.Now.Sub(last) > staleAfter {
			// 2. Decay.
			old := conf
			conf = max(0.0, conf-decayDelta)
			changed = true
			report.Actions = append(report.Actions, Action{"decayed", name,
				fmt.Sprintf("%.1f→%.1f (last_reinforced %s)", old, conf, lastStr)})
		}

		if changed && !opts.DryRun {
			setFM(n.FMNode, "confidence", fmt.Sprintf("%.1f", conf))
			setFM(n.FMNode, "maturity", maturity)
			setFM(n.FMNode, "last_reinforced", last.Format(validate.TimeLayout))
			setFM(n.FMNode, "reinforce_count", strconv.Itoa(count))
			if err := writeNote(abs, n); err != nil {
				return nil, err
			}
		}

		// 3. Archive. Faded check first; both are moves, id survives.
		if conf <= 0 {
			dest := filepath.Join("knowledge/archive/faded", filepath.Base(n.Path))
			report.Actions = append(report.Actions, Action{"archived-faded", name,
				fmt.Sprintf("(confidence %.1f)", conf)})
			if !opts.DryRun {
				if err := moveNote(vaultRoot, n.Path, dest); err != nil {
					return nil, err
				}
			}
			continue
		}
		if age, ok := gitAge(repoRoot, filepath.Join("knowledge-base", n.Path), opts.Now); ok && age > ancientAfter {
			dest := filepath.Join("knowledge/archive", filepath.Base(n.Path))
			report.Actions = append(report.Actions, Action{"archived-ancient", name,
				fmt.Sprintf("(last commit %s ago)", age.Round(24*time.Hour))})
			if !opts.DryRun {
				if err := moveNote(vaultRoot, n.Path, dest); err != nil {
					return nil, err
				}
			}
			continue
		}

		// 4. Graduate (explicit).
		if dest, ok := pick(graduate, n.ID(), n.Path); ok {
			if maturity != "stable" {
				return nil, fmt.Errorf("%s: refusing to graduate non-stable note (maturity %s)", n.Path, maturity)
			}
			if !strings.HasPrefix(dest, "projects/") {
				return nil, fmt.Errorf("%s: graduation destination must be under projects/, got %s", n.Path, dest)
			}
			report.Actions = append(report.Actions, Action{"graduated", name, "→ " + dest})
			if !opts.DryRun {
				for _, f := range []string{"confidence", "maturity", "last_reinforced", "reinforce_count"} {
					deleteFM(n.FMNode, f)
				}
				// Validate against the destination: the note is canon now.
				src := n.Path
				n.Path = dest
				if err := writeNote(abs, n); err != nil {
					return nil, err
				}
				if err := moveNote(vaultRoot, src, dest); err != nil {
					return nil, err
				}
			}
		}
	}

	// Stable candidates the agent may want to graduate next time.
	if !opts.DryRun {
		if err := appendLog(vaultRoot, report); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func appendLog(vaultRoot string, r *Report) error {
	logPath := filepath.Join(vaultRoot, "knowledge/log.md")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n" + r.String())
	return err
}

// writeNote re-serializes frontmatter + body, validating the result before it
// touches disk.
func writeNote(abs string, n *vault.Note) error {
	fmBytes, err := yaml.Marshal(n.FMNode)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := yaml.Unmarshal(fmBytes, &m); err != nil {
		return err
	}
	if errs := validate.Note(n.Path, m, true); len(errs) > 0 {
		return fmt.Errorf("%s: refusing to write invalid frontmatter: %s", n.Path, strings.Join(errs, "; "))
	}
	content := "---\n" + string(fmBytes) + "---\n" + n.Body
	return os.WriteFile(abs, []byte(content), 0o644)
}

func moveNote(vaultRoot, from, to string) error {
	absTo := filepath.Join(vaultRoot, to)
	if err := os.MkdirAll(filepath.Dir(absTo), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(absTo); err == nil {
		return fmt.Errorf("move %s: destination %s already exists", from, to)
	}
	return os.Rename(filepath.Join(vaultRoot, from), absTo)
}

// gitAge returns time since the file's last commit. ok is false for untracked
// files or outside a repo — fs mtime is a lie after checkouts, so we never
// fall back to it.
func gitAge(repoRoot, relPath string, now time.Time) (time.Duration, bool) {
	out, err := exec.Command("git", "-C", repoRoot, "log", "-1", "--format=%ct", "--", relPath).Output()
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, false
	}
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return now.Sub(time.Unix(sec, 0)), true
}

// --- yaml.Node surgery (preserves key order and comments) ---

func mapping(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func setFM(doc *yaml.Node, key, value string) {
	m := mapping(doc)
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].SetString(value)
			// SetString forces !!str; clear the tag so numbers stay numbers.
			m.Content[i+1].Tag = ""
			m.Content[i+1].Style = 0
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value})
}

func deleteFM(doc *yaml.Node, key string) {
	m := mapping(doc)
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// --- small helpers ---

func toSet(xs []string) map[string]bool {
	s := map[string]bool{}
	for _, x := range xs {
		if x != "" {
			s[x] = true
		}
	}
	return s
}

func pick(m map[string]string, keys ...string) (string, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v, true
		}
	}
	return "", false
}

func wikiname(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	}
	return 0
}
