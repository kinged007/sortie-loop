package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed workflows
var embeddedWorkflows embed.FS

// mustMaterialize extracts the embedded workflow files (WORKFLOW.*.md +
// prompts/) to a cache dir on first use and returns the path of name.
// ponytail: cache keyed by binary build; `sortie-loop update-workflows`
// refresh would follow if workflows ever need hot-patching per install.
func mustMaterialize(name string) string {
	dir, err := workflowCacheDir()
	if err != nil {
		fatal(err)
	}
	entries, err := embeddedWorkflows.ReadDir("workflows")
	if err != nil {
		fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		copyEmbedded("workflows/"+e.Name(), filepath.Join(dir, e.Name()))
	}
	prompts, err := embeddedWorkflows.ReadDir("workflows/prompts")
	if err != nil {
		fatal(err)
	}
	for _, p := range prompts {
		copyEmbedded("workflows/prompts/"+p.Name(), filepath.Join(dir, "prompts", p.Name()))
	}
	return filepath.Join(dir, name)
}

func copyEmbedded(src, dest string) {
	data, err := embeddedWorkflows.ReadFile(src)
	if err != nil {
		fatal(err)
	}
	if cur, err := os.ReadFile(dest); err == nil && string(cur) == string(data) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		fatal(err)
	}
}

func workflowCacheDir() (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve cache dir: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "sortie-loop", "workflows-"+sortieVersion), nil
}
