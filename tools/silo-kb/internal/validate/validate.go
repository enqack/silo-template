// Package validate enforces the vault frontmatter contract. It is the single
// implementation shared by the reindex walk, the compile engine's rewrites,
// and the Claude Code PreToolUse hook — so the contract cannot drift.
package validate

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

var maturities = map[string]bool{"seed": true, "developing": true, "stable": true}

// liveStatuses are the truth-states a knowledge note may carry while it is still
// live. `falsified` is not here: falsification is a move into
// knowledge/archive/falsified/, and archived notes are validator-exempt.
var liveStatuses = map[string]bool{"active": true, "disputed": true}

// decayFields are required on knowledge/* notes and forbidden on projects/*.
var decayFields = []string{"confidence", "maturity", "last_reinforced", "reinforce_count"}

// Timestamp layouts accepted in frontmatter: the silo convention, and bare
// dates for date-only fields.
const TimeLayout = "2006-01-02 15:04:05"

func ParseTime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(TimeLayout, s, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("not YYYY-MM-DD HH:MM:SS or YYYY-MM-DD: %q", s)
}

// TimeOf normalizes a frontmatter timestamp value. yaml.v3 decodes unquoted
// timestamps into time.Time, quoted ones stay strings — accept both.
func TimeOf(v any) (time.Time, error) {
	switch x := v.(type) {
	case time.Time:
		return x, nil
	case string:
		return ParseTime(x)
	}
	return time.Time{}, fmt.Errorf("not a timestamp: %v", v)
}

// Note validates the frontmatter contract for a vault-relative path.
// hasFM is false when the file carries no frontmatter block.
// Returned strings are self-correction messages suitable for feeding back to
// an agent verbatim.
func Note(relPath string, fm map[string]any, hasFM bool) []string {
	relPath = path.Clean(strings.TrimPrefix(relPath, "./"))
	base := path.Base(relPath)

	// Reserved names: no frontmatter, except the root index.md which carries
	// exactly okf_version.
	if base == "index.md" || base == "log.md" {
		if relPath == "index.md" {
			if !hasFM || len(fm) != 1 || fm["okf_version"] == nil {
				return []string{"root index.md must have frontmatter containing exactly one field: okf_version"}
			}
			return nil
		}
		if hasFM {
			return []string{fmt.Sprintf("%s is a reserved filename and must have no frontmatter at all", base)}
		}
		return nil
	}

	var errs []string
	if !hasFM {
		return []string{"missing YAML frontmatter; every note needs at least `id` (a UUID, assigned once) and `type`"}
	}

	id, _ := fm["id"].(string)
	if id == "" {
		errs = append(errs, "frontmatter missing required field `id`; generate a UUIDv7 and retry — do not reuse another note's id")
	} else if _, err := uuid.Parse(id); err != nil {
		errs = append(errs, fmt.Sprintf("frontmatter `id` is not a valid UUID: %q", id))
	}
	if t, _ := fm["type"].(string); t == "" {
		errs = append(errs, "frontmatter missing required field `type`")
	}

	tierRoot := strings.SplitN(relPath, "/", 2)[0]
	switch tierRoot {
	case "knowledge":
		if strings.HasPrefix(relPath, "knowledge/archive/") {
			break // archived notes keep whatever fields they faded with
		}
		errs = append(errs, validateKnowledge(fm)...)
	case "projects":
		for _, f := range decayFields {
			if _, present := fm[f]; present {
				errs = append(errs, fmt.Sprintf("projects/* notes are asserted canon and must not carry decay field `%s`; remove it (graduation should have stripped it)", f))
			}
		}
	}
	return errs
}

func validateKnowledge(fm map[string]any) []string {
	var errs []string

	conf, ok := toFloat(fm["confidence"])
	if !ok {
		errs = append(errs, "knowledge/* notes require numeric `confidence` (0.0–1.0)")
	} else if conf < 0 || conf > 1 {
		errs = append(errs, fmt.Sprintf("`confidence` must be within 0.0–1.0, got %v", conf))
	}

	if m, _ := fm["maturity"].(string); !maturities[m] {
		errs = append(errs, "knowledge/* notes require `maturity`: one of seed, developing, stable")
	}

	if lr, present := fm["last_reinforced"]; !present {
		errs = append(errs, "knowledge/* notes require `last_reinforced` (YYYY-MM-DD HH:MM:SS)")
	} else if _, err := TimeOf(lr); err != nil {
		errs = append(errs, fmt.Sprintf("`last_reinforced` %v", err))
	}

	if _, ok := toInt(fm["reinforce_count"]); !ok {
		errs = append(errs, "knowledge/* notes require integer `reinforce_count` (0 for new notes)")
	}

	if srcs, ok := fm["sources"].([]any); !ok || len(srcs) == 0 {
		errs = append(errs, "knowledge/* notes require non-empty `sources` listing provenance (wikilinks to daily/deep-thought notes)")
	}

	// `status` is optional (absent ⇒ active). A note the agent has actively
	// contested is `disputed` — recorded dissent that survives, distinct from a
	// note that merely decayed from neglect.
	if s, present := fm["status"]; present {
		if str, _ := s.(string); !liveStatuses[str] {
			errs = append(errs, "knowledge/* `status`, if set, must be one of: active, disputed (falsification archives the note instead)")
		}
	}
	return errs
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	}
	return 0, false
}
