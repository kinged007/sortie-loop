package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncWorkflowsKeepsEditsFillsGaps(t *testing.T) {
	root := t.TempDir()
	syncWorkflows(root, true)
	dev := filepath.Join(root, ".sortie", "workflows", "WORKFLOW.dev.md")
	if _, err := os.Stat(dev); err != nil {
		t.Fatalf("setup did not install workflows: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md")); err != nil {
		t.Fatalf("setup did not install prompts: %v", err)
	}
	f, _ := os.OpenFile(dev, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n# local tweak")
	f.Close()
	os.Remove(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md"))
	if got := workflowPath(root, "WORKFLOW.dev.md"); got != dev {
		t.Fatalf("workflowPath = %q, want %q", got, dev)
	}
	raw, _ := os.ReadFile(dev)
	if !strings.HasSuffix(string(raw), "\n# local tweak") {
		t.Error("loop run overwrote local workflow edit")
	}
	if _, err := os.Stat(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md")); err != nil {
		t.Error("loop run did not restore missing prompt file")
	}
}
