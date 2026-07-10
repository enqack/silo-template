package compilepass

import (
	"os"
	"os/exec"
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

func TestFalsifyRetainsInPlaceWithReason(t *testing.T) {
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

	// Note stays in place (retained + queryable), NOT moved to archive.
	if _, err := os.Stat(filepath.Join(repo, "knowledge-base/knowledge/archive/falsified/theory.md")); !os.IsNotExist(err) {
		t.Error("falsified note must not be archived — it is retained in place")
	}
	got, err := os.ReadFile(filepath.Join(repo, "knowledge-base/knowledge/concepts/theory.md"))
	if err != nil {
		t.Fatalf("retained note missing: %v", err)
	}
	for _, want := range []string{"status: falsified", "falsified_reason: contradicted by benchmark X", "falsified_at: 2026-07-07 12:00:00"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("retained note missing %q; got:\n%s", want, got)
		}
	}
}

// A falsified note is inert on later runs: it never decays, archives, or
// graduates, and it is not a valid reinforce/graduate target.
func TestFalsifiedNoteIsInert(t *testing.T) {
	repo := t.TempDir()
	id := "de6d5441-c97e-415f-b5ca-0df850ff0d84"
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation log\n")

	// Falsify it.
	if _, err := Run(repo, Options{
		Falsify: map[string]string{id: "wrong"},
		Now:     time.Date(2026, 7, 7, 12, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatal(err)
	}

	// A later run, well past the decay window, must not touch it.
	rep, err := Run(repo, Options{
		Now: time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range rep.Actions {
		if a.Note == "theory" {
			t.Errorf("falsified note should be inert, got action %+v", a)
		}
	}

	// It is no longer a valid reinforce target.
	if _, err := Run(repo, Options{
		Reinforce: []string{id},
		Now:       time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local),
	}); err == nil {
		t.Error("expected error reinforcing a falsified (non-live) note")
	}
}

// --supersede records the replacement as a superseded_by wikilink.
func TestFalsifyWithSupersede(t *testing.T) {
	repo := t.TempDir()
	oldID := "de6d5441-c97e-415f-b5ca-0df850ff0d84"
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	writeVaultNote(t, repo, "knowledge/concepts/better.md", stableNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation log\n")

	if _, err := Run(repo, Options{
		Falsify:   map[string]string{oldID: "superseded"},
		Supersede: map[string]string{oldID: "3f8e2c1a-9b4d-4e6f-8a2b-1c3d5e7f9a0b"},
		Now:       time.Date(2026, 7, 7, 12, 0, 0, 0, time.Local),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(repo, "knowledge-base/knowledge/concepts/theory.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "superseded_by: '[[better]]'") && !strings.Contains(string(got), "superseded_by: \"[[better]]\"") && !strings.Contains(string(got), "superseded_by: [[better]]") {
		t.Errorf("expected superseded_by wikilink to [[better]]; got:\n%s", got)
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

const stableNote = `---
id: 3f8e2c1a-9b4d-4e6f-8a2b-1c3d5e7f9a0b
type: concept
confidence: 0.9
maturity: stable
last_reinforced: 2026-07-07 09:00:00
reinforce_count: 4
sources:
  - "[[2026-07-07]]"
---
A stable theory.
`

// A typo'd id/path must fail the whole run up front — silently doing nothing
// would let the agent report work that never happened.
func TestUnmatchedIDsError(t *testing.T) {
	repo := t.TempDir()
	writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation Log\n")

	_, err := Run(repo, Options{
		Reinforce: []string{"no-such-id"},
		Falsify:   map[string]string{"another-typo": "reason"},
		Now:       time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local),
	})
	if err == nil {
		t.Fatal("expected error for unmatched ids")
	}
	for _, want := range []string{"no live knowledge note matches", "--reinforce no-such-id", "--falsify another-typo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q; got:\n%v", want, err)
		}
	}
}

func TestGraduateRejectsFlatDestination(t *testing.T) {
	repo := t.TempDir()
	id := "3f8e2c1a-9b4d-4e6f-8a2b-1c3d5e7f9a0b"
	writeVaultNote(t, repo, "knowledge/concepts/stable.md", stableNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation Log\n")

	_, err := Run(repo, Options{
		Graduate: map[string]string{id: "projects/flat.md"},
		Now:      time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local),
	})
	if err == nil || !strings.Contains(err.Error(), "projects/<name>/<note>.md") {
		t.Fatalf("flat projects/ destination must be rejected (the indexer never sees it), got %v", err)
	}
}

func TestStableCandidatesReported(t *testing.T) {
	repo := t.TempDir()
	writeVaultNote(t, repo, "knowledge/concepts/stable.md", stableNote)
	writeVaultNote(t, repo, "knowledge/log.md", "# Compilation Log\n")

	rep, err := Run(repo, Options{Now: time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.StableCandidates) != 1 || rep.StableCandidates[0] != "stable" {
		t.Errorf("expected [stable] as graduation candidates, got %v", rep.StableCandidates)
	}
}

// gitCommitAt commits everything in repo with the given commit date.
func gitCommitAt(t *testing.T, repo, date string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "seed", "--date", date},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// A note reinforced in this run must not be archived as ancient in the same
// run, however old its last commit; without reinforcement it is archived.
func TestReinforceShieldsFromAncientArchival(t *testing.T) {
	id := "de6d5441-c97e-415f-b5ca-0df850ff0d84"
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.Local)

	setup := func(t *testing.T) string {
		repo := t.TempDir()
		writeVaultNote(t, repo, "knowledge/concepts/theory.md", seedNote)
		writeVaultNote(t, repo, "knowledge/log.md", "# Compilation Log\n")
		gitCommitAt(t, repo, "2025-12-01T12:00:00")
		return repo
	}

	t.Run("unreinforced ancient note is archived", func(t *testing.T) {
		repo := setup(t)
		rep, err := Run(repo, Options{Now: now})
		if err != nil {
			t.Fatal(err)
		}
		var kinds []string
		for _, a := range rep.Actions {
			kinds = append(kinds, a.Kind)
		}
		if !strings.Contains(strings.Join(kinds, " "), "archived-ancient") {
			t.Fatalf("expected archived-ancient, got %v", kinds)
		}
	})

	t.Run("reinforced note survives", func(t *testing.T) {
		repo := setup(t)
		rep, err := Run(repo, Options{Reinforce: []string{id}, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range rep.Actions {
			if a.Kind == "archived-ancient" {
				t.Fatalf("reinforced note must not be ancient-archived, got %+v", rep.Actions)
			}
		}
		if _, err := os.Stat(filepath.Join(repo, "knowledge-base/knowledge/concepts/theory.md")); err != nil {
			t.Errorf("reinforced note should still be live: %v", err)
		}
	})
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
