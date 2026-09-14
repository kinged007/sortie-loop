package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed workflows
var embeddedWorkflows embed.FS

// syncWorkflows installs the embedded workflow files (WORKFLOW.*.md +
// prompts/) into <root>/.sortie/workflows/ so they sit visible next to
// the loop's config, env, and workspaces. With refresh=false only missing
// files are written, leaving edits alone; setup passes refresh=true to
// bring everything back in line with the binary.
// ponytail: no version stamp; re-run `sortie-loop setup` after upgrading
// to refresh, or delete .sortie/workflows for a clean reinstall.
func syncWorkflows(root string, refresh bool) {
	entries, err := embeddedWorkflows.ReadDir("workflows")
	if err != nil {
		fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		writeWorkflow(filepath.Join(root, ".sortie", "workflows", e.Name()), "workflows/"+e.Name(), refresh)
	}
	prompts, err := embeddedWorkflows.ReadDir("workflows/prompts")
	if err != nil {
		fatal(err)
	}
	for _, p := range prompts {
		writeWorkflow(filepath.Join(root, ".sortie", "workflows", "prompts", p.Name()), "workflows/prompts/"+p.Name(), refresh)
	}
}

func writeWorkflow(dest, src string, refresh bool) {
	data, err := embeddedWorkflows.ReadFile(src)
	if err != nil {
		fatal(err)
	}
	if cur, err := os.ReadFile(dest); err == nil {
		if string(cur) == string(data) || !refresh {
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

// workflowPath returns the repo-local workflow file, filling gaps from the
// embedded copies so checkouts set up before workflows moved into .sortie
// still run.
func workflowPath(root, name string) string {
	syncWorkflows(root, false)
	return filepath.Join(root, ".sortie", "workflows", name)
}

// discoverWorkflows lists the installed WORKFLOW.*.md files in name order,
// filling gaps from the embedded copies first. The startup set is
// whatever files exist on disk, so adding WORKFLOW.triage.md (embedded
// or hand-written) starts a triage loop with no code change — the loop
// name is the filename stem ("triage").
func discoverWorkflows(root string) []struct{ name, file string } {
	syncWorkflows(root, false)
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
