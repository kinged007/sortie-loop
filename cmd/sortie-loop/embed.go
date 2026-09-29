package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
		if policy == keepEdits || !confirmOverwrite(displayPath(dest)) {
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

// confirmOverwrite asks before replacing something a person may have
// changed: a hand-edited workflow file, or an existing label whose colour
// or description differs. what is the text to show. Only an explicit yes
// proceeds, so a closed, piped, or non-interactive stdin declines instead
// of guessing.
func confirmOverwrite(what string) bool {
	fmt.Printf("  %s differs from what sortie-loop ships. Overwrite? [y/N] ", what)
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
	fmt.Printf("  kept %s — delete it and re-run setup to take the shipped version\n", what)
	return false
}

// displayPath shortens a path for a prompt, preferring a path relative to
// the current directory over a long absolute one.
func displayPath(dest string) string {
	if rel, err := filepath.Rel(".", dest); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return dest
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
func discoverWorkflows(root string) []loopDef {
	syncWorkflows(root, keepEdits)
	entries, err := os.ReadDir(filepath.Join(root, ".sortie", "workflows"))
	if err != nil {
		fatal(err)
	}
	var loops []loopDef
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "WORKFLOW.") || !strings.HasSuffix(name, ".md") {
			continue
		}
		stem := strings.TrimSuffix(strings.TrimPrefix(name, "WORKFLOW."), ".md")
		loops = append(loops, loopDef{stem, name})
	}
	return loops
}

// loopDef is one loop: its name (the WORKFLOW.*.md stem) and the file
// sortie runs it from.
type loopDef struct{ name, file string }

// selectLoops narrows the discovered set to the comma-separated names
// in only, so a supervisor can start a chosen subset of a repo's
// loops. An empty only keeps them all, which is what a plain run does.
// A name matching no installed workflow is an error rather than a
// silent omission: the caller asked for a loop that does not exist, and
// quietly starting the rest would leave that work unwatched.
func selectLoops(loops []loopDef, only string) ([]loopDef, error) {
	if strings.TrimSpace(only) == "" {
		return loops, nil
	}
	keep := map[string]bool{}
	for _, n := range strings.Split(only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			keep[n] = true
		}
	}
	if len(keep) == 0 {
		return nil, fmt.Errorf("SORTIE_LOOP_ONLY=%q names no workflow", only)
	}
	var out []loopDef
	for _, l := range loops {
		if keep[l.name] {
			out = append(out, l)
			delete(keep, l.name)
		}
	}
	if len(keep) > 0 {
		unknown := make([]string, 0, len(keep))
		for n := range keep {
			unknown = append(unknown, n)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("no WORKFLOW.*.md named %s in this repo", strings.Join(unknown, " or "))
	}
	return out, nil
}
