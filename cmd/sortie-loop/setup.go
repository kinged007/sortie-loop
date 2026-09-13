package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// labels created by `sortie-loop setup`: name, color, description.
var labels = [][3]string{
	{"agent:quick", "fbca04", "Track: small change, merged to main, no PR"},
	{"agent:plan-needed", "d876e3", "Track: plan must be written and approved first"},
	{"agent:build", "1d76db", "Track: full development, lands via PR"},
	{"backlog", "e4e669", "State: queued, not started"},
	{"in-progress", "1d76db", "State: work in progress"},
	{"review", "5319e7", "State: ready for human review"},
	{"done", "0e8a16", "State: completed"},
	{"needs-human", "d73a4a", "Escalation: agent needs a person"},
	{"agent:review", "5319e7", "Command: request a review of a Sortie-managed PR"},
	{"agent:fix", "5319e7", "Command: apply review feedback on a Sortie-managed PR"},
	{"agent:needs-review", "fbca04", "State: PR is waiting for agent review"},
	{"agent:reviewed", "5319e7", "State: agent review posted"},
	{"agent:review-complete", "0e8a16", "State: review feedback addressed"},
}

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
	// Refuse to run inside the sortie-loop checkout itself: setup belongs
	// in the target repo, and running here would pollute this repo.
	if _, err := os.Stat(filepath.Join(abs, "cmd", "sortie-loop", "main.go")); err == nil {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			fatal(fmt.Errorf("refusing to set up inside the sortie-loop checkout itself; cd to the target repo first"))
		}
	}
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
			"milestone: \"\"\n"
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
			fatal(err)
		}
		fmt.Println("created", cfgPath)
	}
	repo := configRepo(abs)
	ensureGitignore(abs)
	for _, l := range labels {
		create := exec.Command("gh", "label", "create", l[0], "--repo", repo,
			"--color", l[1], "--description", l[2])
		if _, err := create.CombinedOutput(); err != nil {
			edit := exec.Command("gh", "label", "edit", l[0], "--repo", repo,
				"--color", l[1], "--description", l[2])
			if out, err := edit.CombinedOutput(); err != nil {
				fatal(fmt.Errorf("label %s: %s: %w", l[0], string(out), err))
			}
		}
	}
	fmt.Println("labels synced to", repo)
}

// ensureGitignore appends the loop's local-state entries to .gitignore,
// creating the file if missing. Existing content is left untouched.
func ensureGitignore(dir string) {
	want := []string{".sortie/.env.loop", ".sortie/workspaces/", ".sortie-*.db", "sortie-loop"}
	path := filepath.Join(dir, ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		fatal(fmt.Errorf("read .gitignore: %w", err))
	}
	have := map[string]bool{}
	for _, line := range strings.Split(string(raw), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var missing []string
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) == 0 {
		return
	}
	out := strings.TrimRight(string(raw), "\n")
	if len(out) > 0 {
		out += "\n"
	}
	out += "# sortie-loop local state (added by sortie-loop setup)\n"
	for _, m := range missing {
		out += m + "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		fatal(fmt.Errorf("write .gitignore: %w", err))
	}
	fmt.Println("updated", path)
}
