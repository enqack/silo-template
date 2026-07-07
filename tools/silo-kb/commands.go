package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"silo.local/silo-kb/internal/compilepass"
	"silo.local/silo-kb/internal/embed"
	"silo.local/silo-kb/internal/indexgen"
	"silo.local/silo-kb/internal/mcpserver"
	"silo.local/silo-kb/internal/query"
	"silo.local/silo-kb/internal/store"
	"silo.local/silo-kb/internal/validate"
	"silo.local/silo-kb/internal/vault"
)

func vaultRoot() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "knowledge-base"), nil
}

func migrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply the derived-index schema idempotently",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pool, err := store.Connect(ctx)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := store.Migrate(ctx, pool); err != nil {
				return err
			}
			fmt.Println("schema up to date")
			return nil
		},
	}
}

func reindexCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Delta-sync the vault into Postgres (chunks + embeddings)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			vr, err := vaultRoot()
			if err != nil {
				return err
			}
			notes, err := vault.Walk(vr)
			if err != nil {
				return err
			}
			pool, err := store.Connect(ctx)
			if err != nil {
				return err
			}
			defer pool.Close()
			stats, err := store.Reindex(ctx, pool, embed.New(""), notes, full)
			if err != nil {
				return err
			}
			fmt.Printf("notes: %d (%d unchanged, %d moved, %d pruned)\nchunks: %d embedded, %d unchanged\n",
				stats.Notes, stats.SkippedNotes, stats.MovedNotes, stats.NotesPruned,
				stats.ChunksEmbedded, stats.ChunksKept)
			return nil
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "truncate the index and rebuild everything")
	return cmd
}

func queryCmd() *cobra.Command {
	var project string
	var topK int
	cmd := &cobra.Command{
		Use:   "query <text>",
		Short: "Hybrid RRF retrieval over the full corpus",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			pool, err := store.Connect(ctx)
			if err != nil {
				return err
			}
			defer pool.Close()
			vec, err := embed.New("").Query(ctx, args[0])
			if err != nil {
				return err
			}
			results, err := query.Run(ctx, pool, vec, args[0], project, topK)
			if err != nil {
				return err
			}
			fmt.Println(mcpserver.Format(results))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "restrict to one project")
	cmd.Flags().IntVar(&topK, "top-k", query.DefaultTopK, "number of fused results")
	return cmd
}

func compileCmd() *cobra.Command {
	var reinforce []string
	var graduate []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "compile",
		Short: "Run the knowledge lifecycle: reinforce, decay, archive, graduate",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findRepoRoot()
			if err != nil {
				return err
			}
			grad := map[string]string{}
			for _, g := range graduate {
				k, v, ok := strings.Cut(g, ":")
				if !ok {
					return fmt.Errorf("--graduate wants <id-or-path>:<projects/...>, got %q", g)
				}
				grad[k] = v
			}
			report, err := compilepass.Run(root, compilepass.Options{
				Reinforce: reinforce,
				Graduate:  grad,
				DryRun:    dryRun,
			})
			if err != nil {
				return err
			}
			fmt.Print(report.String())
			if dryRun {
				fmt.Println("(dry run — nothing written)")
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&reinforce, "reinforce", nil, "note ids or vault-relative paths confirmed by this run")
	cmd.Flags().StringSliceVar(&graduate, "graduate", nil, "<id-or-path>:<projects/dest.md> — move a stable article to canon")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report without writing")
	return cmd
}

func syncIndexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-index",
		Short: "Regenerate knowledge-base/knowledge/index.md",
		RunE: func(cmd *cobra.Command, args []string) error {
			vr, err := vaultRoot()
			if err != nil {
				return err
			}
			if err := indexgen.Write(vr); err != nil {
				return err
			}
			fmt.Println("knowledge/index.md regenerated")
			return nil
		},
	}
}

func injectIndexCmd() *cobra.Command {
	var budget int
	cmd := &cobra.Command{
		Use:   "inject-index",
		Short: "Emit the index truncated to a context budget (for SessionStart hooks)",
		RunE: func(cmd *cobra.Command, args []string) error {
			vr, err := vaultRoot()
			if err != nil {
				return err
			}
			content, err := indexgen.Generate(vr)
			if err != nil {
				return err
			}
			fmt.Println(indexgen.Truncate(content, budget))
			return nil
		},
	}
	cmd.Flags().IntVar(&budget, "budget", 4000, "character budget (0 = unlimited)")
	return cmd
}

func serveMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve-mcp",
		Short: "Serve query_knowledge over stdio MCP",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpserver.Run(cmd.Context())
		},
	}
}

// --- validate: CLI mode and Claude Code PreToolUse hook mode ---

func validateCmd() *cobra.Command {
	var hookStdin bool
	cmd := &cobra.Command{
		Use:   "validate [files...]",
		Short: "Validate vault frontmatter contracts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if hookStdin {
				runHook() // handles its own exit codes; never returns an error
				return nil
			}
			vr, err := vaultRoot()
			if err != nil {
				return err
			}
			if _, err := vault.Walk(vr); err != nil {
				return err
			}
			fmt.Println("vault valid")
			return nil
		},
	}
	cmd.Flags().BoolVar(&hookStdin, "hook-stdin", false, "read a Claude Code PreToolUse hook payload from stdin")
	return cmd
}

// hookInput is the subset of the Claude Code PreToolUse payload we need.
type hookInput struct {
	ToolName  string `json:"tool_name"`
	ToolInput struct {
		FilePath  string `json:"file_path"`
		Content   string `json:"content"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"tool_input"`
	CWD string `json:"cwd"`
}

// runHook validates a pending Write/Edit against the frontmatter contract.
// Exit 0 allows, exit 2 blocks with stderr fed back to the model. Internal
// errors fail open (exit 0) so a broken validator never wedges unrelated
// writes.
func runHook() {
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		os.Exit(0)
	}
	path := in.ToolInput.FilePath
	if path == "" || !strings.HasSuffix(path, ".md") {
		os.Exit(0)
	}

	// Resolve the vault-relative path; only knowledge/ and projects/ are guarded.
	idx := strings.Index(filepath.ToSlash(path), "knowledge-base/")
	if idx < 0 {
		os.Exit(0)
	}
	rel := filepath.ToSlash(path)[idx+len("knowledge-base/"):]
	if !strings.HasPrefix(rel, "knowledge/") && !strings.HasPrefix(rel, "projects/") {
		os.Exit(0)
	}

	// The generated index is off-limits regardless of content.
	if rel == "knowledge/index.md" {
		fmt.Fprintln(os.Stderr, "knowledge-base/knowledge/index.md is generated — do not hand-edit it; run /kb-sync-index (silo-kb sync-index) instead")
		os.Exit(2)
	}

	// Reconstruct the resulting file content.
	var content []byte
	switch in.ToolName {
	case "Write":
		content = []byte(in.ToolInput.Content)
	case "Edit", "MultiEdit":
		current, err := os.ReadFile(path)
		if err != nil {
			os.Exit(0) // new file via Edit shouldn't happen; fail open
		}
		if in.ToolInput.OldString == "" {
			os.Exit(0)
		}
		content = []byte(strings.Replace(string(current), in.ToolInput.OldString, in.ToolInput.NewString, 1))
	default:
		os.Exit(0)
	}

	n, err := vault.ParseNote(rel, content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid YAML frontmatter: %v — fix the YAML and retry\n", err)
		os.Exit(2)
	}
	if errs := validate.Note(rel, n.Frontmatter, n.Frontmatter != nil); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "frontmatter contract violation in %s:\n", rel)
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
		os.Exit(2)
	}
	os.Exit(0)
}
