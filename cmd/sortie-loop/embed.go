package main

import (
	"embed"
	"os"
	"path/filepath"
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
