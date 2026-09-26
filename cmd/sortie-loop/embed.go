package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed workflows
var embeddedWorkflows embed.FS

//go:embed pm-agent.md
var embeddedPMAgent embed.FS

// writePolicy decides what syncWorkflows does when an installed file
// already exists and differs from the embedded copy. Installed workflows
// and prompts are hand-edited, so setup confirms before replacing one and
// the loop startup path keeps edits because it must never block on a
// prompt.
type writePolicy int

const (
	keepEdits writePolicy = iota
	askBeforeOverwrite
)

// promptIn wraps stdin so answers typed ahead are not dropped between
// per-file prompts by a reader rebuilt each time.
var promptIn *bufio.Reader

// syncWorkflows installs the embedded workflow files (WORKFLOW.*.md +
// prompts/) into <root>/.sortie/workflows/ so they sit visible next to
// the loop's config, env, and workspaces. A missing file is always
// written. A file that exists and differs is a hand edit: kept under
// keepEdits, and under askBeforeOverwrite replaced only once confirmed.
// ponytail: no version stamp, so a file left untouched but outdated by a
// binary upgrade is never noticed; delete it and re-run setup.
func syncWorkflows(root string, policy writePolicy) {
	entries, err := embeddedWorkflows.ReadDir("workflows")
	if err != nil {
		fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		writeWorkflow(embeddedWorkflows, filepath.Join(root, ".sortie", "workflows", e.Name()), "workflows/"+e.Name(), policy)
	}
	prompts, err := embeddedWorkflows.ReadDir("workflows/prompts")
	if err != nil {
		fatal(err)
	}
	for _, p := range prompts {
		writeWorkflow(embeddedWorkflows, filepath.Join(root, ".sortie", "workflows", "prompts", p.Name()), "workflows/prompts/"+p.Name(), policy)
	}
}

// syncPMAgent installs the project-manager prompt into <root>/.sortie/
// beside config.yaml, so every repo that runs setup gets a PM ready to
// use. Hand-edited copies follow the same rules as the workflows.
func syncPMAgent(root string, policy writePolicy) {
	writeWorkflow(embeddedPMAgent, filepath.Join(root, ".sortie", "pm-agent.md"), "pm-agent.md", policy)
}

func writeWorkflow(fsys fs.FS, dest, src string, policy writePolicy) {
	data, err := fs.ReadFile(fsys, src)
	if err != nil {
		fatal(err)
	}
	if cur, err := os.ReadFile(dest); err == nil {
		if string(cur) == string(data) {
			return
		}
		if policy == keepEdits || !confirmOverwrite(dest) {
			return
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		fatal(err)
	}
}

// confirmOverwrite asks before replacing a hand-edited file. Only an
// explicit yes proceeds, so a closed, piped, or non-interactive stdin
// declines instead of guessing.
func confirmOverwrite(dest string) bool {
	shown := dest
	if rel, err := filepath.Rel(".", dest); err == nil {
		shown = rel
	}
	fmt.Printf("  %s differs from the shipped copy. Overwrite? [y/N] ", shown)
	if promptIn == nil {
		promptIn = bufio.NewReader(os.Stdin)
	}
	line, err := promptIn.ReadString('\n')
	if err != nil && line == "" {
		fmt.Println()
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	fmt.Printf("  kept %s — delete it and re-run setup to take the shipped copy\n", shown)
	return false
}

// workflowPath returns the repo-local workflow file, filling gaps from the
// embedded copies so checkouts set up before workflows moved into .sortie
// still run.
func workflowPath(root, name string) string {
	syncWorkflows(root, keepEdits)
	return filepath.Join(root, ".sortie", "workflows", name)
}

// discoverWorkflows lists the installed WORKFLOW.*.md files in name order,
// filling gaps from the embedded copies first. The startup set is
// whatever files exist on disk, so adding WORKFLOW.triage.md (embedded
// or hand-written) starts a triage loop with no code change — the loop
// name is the filename stem ("triage").
func discoverWorkflows(root string) []struct{ name, file string } {
	syncWorkflows(root, keepEdits)
	entries, err := os.ReadDir(filepath.Join(root, ".sortie", "workflows"))
	if err != nil {
		fatal(err)
	}
	var loops []struct{ name, file string }
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "WORKFLOW.") || !strings.HasSuffix(name, ".md") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "WORKFLOW."), ".md")
		loops = append(loops, struct{ name, file string }{stem, name})
	}
	return loops
}
