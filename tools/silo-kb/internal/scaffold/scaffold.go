// Package scaffold writes the fresh-silo knowledge-base/ skeleton: the empty
// vault layout a brand-new silo starts from. It is the canonical Go owner of
// that layout; `silo-kb reset` uses it to restore a clean template.
//
// IMPORTANT: the directory set and the three template files below must stay
// byte-for-byte in sync with the `silo-init` scaffold heredocs in flake.nix
// (the bootstrap-time copy, used before this binary exists on a fresh silo).
package scaffold

import (
	_ "embed"
	"os"
	"path/filepath"
)

// tierDirs get a .gitkeep so the empty tree survives a git commit. Mirrors the
// mkdir set in flake.nix's silo-init.
var tierDirs = []string{
	"daily",
	"deep-thoughts",
	"knowledge/concepts",
	"knowledge/cursed-knowledge",
	"knowledge/lessons-learned",
	"knowledge/archive/faded",
	"knowledge/archive/falsified",
	"projects",
}

//go:embed templates/root-index.md
var rootIndex []byte

//go:embed templates/knowledge-index.md
var knowledgeIndex []byte

//go:embed templates/knowledge-log.md
var knowledgeLog []byte

// files maps a vault-relative path to its scaffold content.
var files = map[string][]byte{
	"index.md":           rootIndex,
	"knowledge/index.md": knowledgeIndex,
	"knowledge/log.md":   knowledgeLog,
}

// Write lays down the fresh knowledge-base/ skeleton under repoRoot: the tier
// directories (each with a .gitkeep) and the three reserved files. It assumes
// knowledge-base/ does not already exist (reset removes it first); MkdirAll is
// idempotent, but existing authored files are not touched.
func Write(repoRoot string) error {
	vr := filepath.Join(repoRoot, "knowledge-base")
	for _, d := range tierDirs {
		full := filepath.Join(vr, d)
		if err := os.MkdirAll(full, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(full, ".gitkeep"), nil, 0o644); err != nil {
			return err
		}
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(vr, rel), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}
