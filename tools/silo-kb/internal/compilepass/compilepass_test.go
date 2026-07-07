package compilepass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeVaultNote lays down a knowledge note under a temp repo's vault.
func writeVaultNote(t *testing.T, repo, relUnderVault, content string) {
	t.Helper()
	abs := filepath.Join(repo, "knowledge-base", relUnderVault)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const seedNote = `---
id: de6d5441-c97e-415f-b5ca-0df850ff0d84
type: concept
confidence: 0.7
maturity: seed
last_reinforced: 2026-07-07 09:00:00
reinforce_count: 0
sources:
  - "[[2026-07-07]]"
---
A working theory.
`

func TestFalsifyArchivesWithReason(t *testing.T) {
	repo := t.TempDir()
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	// log.md must exist for the audit append.
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation log\n")

	rep, err := Run(repo, Options{
		Falsify: map[string]string{"de6d5441-c97e-415f-b5ca-0df850ff0d84": "contradicted by benchmark X"},
		Now:     time.Date(2026, 7, 7, 12, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(rep.Actions) != 1 || rep.Actions[0].Kind != "falsified" {
		t.Fatalf("expected one falsified action, got %+v", rep.Actions)
	}

	// Original gone, archived copy present with reason + status.
	if _, err := os.Stat(filepath.Join(repo, "knowledge-base/knowledge/concepts/theory.md")); !os.IsNotExist(err) {
		t.Error("original note should have been moved")
	}
	got, err := os.ReadFile(filepath.Join(repo, "knowledge-base/knowledge/archive/falsified/theory.md"))
	if err != nil {
		t.Fatalf("archived note missing: %v", err)
	}
	for _, want := range []string{"status: falsified", "falsified_reason: contradicted by benchmark X", "falsified_at: 2026-07-07 12:00:00"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("archived note missing %q; got:\n%s", want, got)
		}
	}
}

func TestFalsifyWinsOverReinforce(t *testing.T) {
	repo := t.TempDir()
	id := "de6d5441-c97e-415f-b5ca-0df850ff0d84"
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation log\n")

	rep, err := Run(repo, Options{
		Reinforce: []string{id},
		Falsify:   map[string]string{id: "wrong"},
		Now:       time.Date(2026, 7, 7, 12, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Actions) != 1 || rep.Actions[0].Kind != "falsified" {
		t.Fatalf("falsify should win over reinforce, got %+v", rep.Actions)
	}
}

func TestFalsifyRequiresReason(t *testing.T) {
	repo := t.TempDir()
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation log\n")

	_, err := Run(repo, Options{
		Falsify: map[string]string{"de6d5441-c97e-415f-b5ca-0df850ff0d84": "  "},
		Now:     time.Date(2026, 7, 7, 12, 0, 0, 0, time.Local),
	})
	if err == nil {
		t.Fatal("falsify with blank reason should error")
	}
}
