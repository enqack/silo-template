package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantFM     string
		wantBody   string
		wantHasFM  bool
	}{
		{"with fm", "---\nid: x\n---\nbody\n", "id: x", "body\n", true},
		{"no fm", "just body\n", "", "just body\n", false},
		{"hr not fm", "body\n\n---\n\nmore\n", "", "body\n\n---\n\nmore\n", false},
		{"empty body", "---\nid: x\n---\n", "id: x", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fm, body, hasFM := SplitFrontmatter([]byte(c.in))
			if fm != c.wantFM || body != c.wantBody || hasFM != c.wantHasFM {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", fm, body, hasFM, c.wantFM, c.wantBody, c.wantHasFM)
			}
		})
	}
}

func TestTierOf(t *testing.T) {
	cases := []struct {
		path    string
		tier    Tier
		project string
		ok      bool
	}{
		{"daily/2026-07-07.md", TierDaily, "", true},
		{"deep-thoughts/x.md", TierDeepThought, "", true},
		{"knowledge/concepts/foo.md", TierKnowledge, "", true},
		{"projects/silo-kb/overview.md", TierProject, "silo-kb", true},
		{"projects/orphan.md", TierProject, "", false},
		{"index.md", "", "", false},
	}
	for _, c := range cases {
		tier, project, ok := TierOf(c.path)
		if tier != c.tier || project != c.project || ok != c.ok {
			t.Errorf("TierOf(%q) = (%v, %q, %v), want (%v, %q, %v)", c.path, tier, project, ok, c.tier, c.project, c.ok)
		}
	}
}

// TestWalkRealVault walks the repo's actual knowledge-base as a fixture: the
// seed vault must always validate.
func TestWalkRealVault(t *testing.T) {
	root := findVault(t)
	notes, err := Walk(root)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(notes) < 4 {
		t.Fatalf("expected at least 4 seed notes, got %d", len(notes))
	}
	ids := map[string]string{}
	for _, n := range notes {
		if n.ID() == "" {
			t.Errorf("%s: empty id", n.Path)
		}
		if prev, dup := ids[n.ID()]; dup {
			t.Errorf("duplicate id %s: %s and %s", n.ID(), prev, n.Path)
		}
		ids[n.ID()] = n.Path
		if n.ContentHash == "" {
			t.Errorf("%s: empty content hash", n.Path)
		}
	}
}

func TestWalkRejectsContractViolations(t *testing.T) {
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "knowledge/concepts"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "knowledge/concepts/bad.md"),
		[]byte("---\ntype: concept\n---\n\nno id, no decay fields\n"), 0o644))

	_, err := Walk(dir)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	for _, want := range []string{"`id`", "confidence", "maturity"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s; got:\n%v", want, err)
		}
	}
}

func findVault(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, "knowledge-base")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("knowledge-base not found above test dir")
	return ""
}
