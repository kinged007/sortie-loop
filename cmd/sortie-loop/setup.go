package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kinged007/sortie-loop/internal/config"
)

// runSetup creates .sortie/config.yaml if missing, syncs labels via gh,
// and ensures .gitignore covers the loop's local state.
// The --repo flag value is a repo slug, never a directory.
func runSetup(dir string) {
	if f := detectRepoFlag(); f != "" && dir == f {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal(err)
	}
	// Dogfooding is supported: setup runs in this checkout like any other
	// repo. Everything it writes lands under .sortie/, which
	// ensureGitignore keeps untracked.
	if _, err := exec.LookPath("gh"); err != nil {
		fatal(fmt.Errorf("gh CLI not found"))
	}
	cfgPath := filepath.Join(abs, ".sortie", "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		repo := detectRepoFlag()
		if repo == "" {
			repo = guessRepo(abs)
		}
		body := "# Repo id (owner/name). Empty = detect from git remote at runtime.\n" +
			"repo: " + repo + "\n" +
			"# Token for the tracker. Empty = GITHUB_TOKEN/GH_TOKEN env.\n" +
			"token: \"\"\n" +
			"# Optional milestone title to restrict all loops to (empty = off).\n" +
			"milestone: \"\"\n" +
			"# Assignee scope for all loops (@me = the token owner; only\n" +
			"# items assigned to that user are picked up). Set to \"\"\n" +
			"# to disable (shared backlog).\n" +
			"assignee: \"@me\"\n" +
			"# Per-loop query-filter overrides, keyed by WORKFLOW.*.md stem.\n" +
			"# New loops start on the default filter; narrow them here, e.g.:\n" +
			"#filters:\n" +
			"#  triage: \"label:agent:triage -label:needs-human\"\n"
		labelsBody := ""
		for _, l := range config.DefaultLabels {
			labelsBody += fmt.Sprintf("- {name: %q, color: %q, description: %q}\n", l.Name, l.Color, l.Description)
		}
		body += "# GitHub labels setup ensures exist (create-or-edit; others left alone).\n" +
			"# Add a custom loop's labels here, then re-run setup.\n" +
			"labels:\n" + labelsBody
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
			fatal(err)
		}
		fmt.Println("created", cfgPath)
	}
	syncWorkflows(abs, askBeforeOverwrite)
	fmt.Println("workflows ready in", filepath.Join(abs, ".sortie", "workflows"))
	syncPMAgent(abs, askBeforeOverwrite)
	fmt.Println("project manager prompt ready in", filepath.Join(abs, ".sortie", "pm-agent.md"))
	ensureSortieLink()
	repo := configRepo(abs)
	ensureGitignore(abs)
	// Labels come from the config file so custom loops can add their own:
	// list entries under `labels:`, re-run setup, and they are created.
	// Sync is additive — labels absent from the list are left alone.
	cfg, err := config.Load(abs)
	if err != nil {
		fatal(err)
	}
	for _, l := range cfg.Labels {
		create := exec.Command("gh", "label", "create", l.Name, "--repo", repo,
			"--color", l.Color, "--description", l.Description)
		if _, err := create.CombinedOutput(); err != nil {
			edit := exec.Command("gh", "label", "edit", l.Name, "--repo", repo,
				"--color", l.Color, "--description", l.Description)
			if out, err := edit.CombinedOutput(); err != nil {
				fatal(fmt.Errorf("label %s: %s: %w", l.Name, string(out), err))
			}
		}
	}
	fmt.Println("labels synced to", repo)
}

// oldGitignoreEntries is the granular scheme setup wrote before ignoring
// .sortie/ wholesale; dropped as subsumed when seen.
var oldGitignoreEntries = []string{
	".sortie/.env.loop", ".sortie/workflows/", ".sortie/workspaces/", ".sortie-*.db",
}

// selectiveGitignore replaces `.sortie/` for teams that share loop config
// in git. Only track config.yaml with `token: ""` — secrets stay in env.
// (`.sortie/*` rather than `.sortie/` so the `!` re-includes take effect:
// git never descends into an excluded directory.)
const selectiveGitignore = `# sortie-loop: track shared config, ignore run state
.sortie/*
!.sortie/config.yaml
!.sortie/workflows/
.sortie-*.db`

// ensureGitignore ignores .sortie/ wholesale — generated env, workspaces,
// db files, and local config all stay untracked — creating .gitignore if
// missing. Stale lines from the old granular scheme are removed. When the
// file re-includes parts of .sortie/ via negations the user tracks
// selectively on purpose, so sortie lines are left alone.
func ensureGitignore(dir string) {
	path := filepath.Join(dir, ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		fatal(fmt.Errorf("read .gitignore: %w", err))
	}
	var selective bool
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		t := strings.TrimSpace(line)
		have[t] = true
		if strings.HasPrefix(t, "!") && strings.Contains(t, ".sortie") {
			selective = true
		}
	}
	if selective {
		fmt.Println(".gitignore tracks parts of .sortie/ — leaving ignore rules alone")
		return
	}
	want := []string{".sortie/", "sortie-loop"}
	old := map[string]bool{}
	for _, e := range oldGitignoreEntries {
		old[e] = true
	}
	var kept []string
	var dropped bool
	for _, line := range strings.Split(string(raw), "\n") {
		if old[strings.TrimSpace(line)] {
			dropped = true
			continue
		}
		kept = append(kept, line)
	}
	var missing []string
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) == 0 && !dropped {
		fmt.Println(".sortie/ is fully ignored")
		return
	}
	out := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if len(out) > 0 {
		out += "\n"
	}
	if !have["# sortie-loop local state (added by sortie-loop setup)"] {
		out += "# sortie-loop local state (added by sortie-loop setup)\n"
	}
	for _, m := range missing {
		out += m + "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		fatal(fmt.Errorf("write .gitignore: %w", err))
	}
	fmt.Println("updated", path, "— .sortie/ is now fully ignored")
	fmt.Println("To share loop config in git instead, replace `.sortie/` with:\n" + selectiveGitignore)
}
