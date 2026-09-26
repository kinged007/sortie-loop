package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kinged007/sortie-loop/internal/config"
)

func TestSetupSeedsLabelsIntoFreshConfig(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".sortie"), 0o755)
	// Simulate the config body setup writes for a fresh repo, then check
	// it parses and carries every default label.
	var body string
	for _, l := range config.DefaultLabels {
		body += "- {name: \"" + l.Name + "\", color: \"" + l.Color + "\", description: \"x\"}\n"
	}
	os.WriteFile(filepath.Join(root, ".sortie", "config.yaml"),
		[]byte("repo: o/r\ntoken: tok\nlabels:\n"+body), 0o644)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Labels) != len(config.DefaultLabels) {
		t.Fatalf("labels = %d, want %d", len(cfg.Labels), len(config.DefaultLabels))
	}
	if cfg.Labels[0].Name != config.DefaultLabels[0].Name {
		t.Errorf("first label = %q", cfg.Labels[0].Name)
	}
}

func TestSyncWorkflowsKeepsEditsFillsGaps(t *testing.T) {
	root := t.TempDir()
	syncWorkflows(root, keepEdits)
	build := filepath.Join(root, ".sortie", "workflows", "WORKFLOW.build.md")
	if _, err := os.Stat(build); err != nil {
		t.Fatalf("setup did not install workflows: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md")); err != nil {
		t.Fatalf("setup did not install prompts: %v", err)
	}
	f, _ := os.OpenFile(build, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n# local tweak")
	f.Close()
	os.Remove(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md"))
	if got := workflowPath(root, "WORKFLOW.build.md"); got != build {
		t.Fatalf("workflowPath = %q, want %q", got, build)
	}
	raw, _ := os.ReadFile(build)
	if !strings.HasSuffix(string(raw), "\n# local tweak") {
		t.Error("loop run overwrote local workflow edit")
	}
	if _, err := os.Stat(filepath.Join(root, ".sortie", "workflows", "prompts", "quick.md")); err != nil {
		t.Error("loop run did not restore missing prompt file")
	}
}

func TestSyncWorkflowsAsksBeforeOverwrite(t *testing.T) {
	root := t.TempDir()
	syncWorkflows(root, keepEdits)
	build := filepath.Join(root, ".sortie", "workflows", "WORKFLOW.build.md")
	const mine = "---\n# hand edited\n"
	if err := os.WriteFile(build, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	// No answer on stdin: the edit survives setup.
	withStdin(t, "", func() { syncWorkflows(root, askBeforeOverwrite) })
	if got, _ := os.ReadFile(build); string(got) != mine {
		t.Error("setup replaced a customized workflow without confirmation")
	}

	// An explicit no, likewise.
	withStdin(t, "n\n", func() { syncWorkflows(root, askBeforeOverwrite) })
	if got, _ := os.ReadFile(build); string(got) != mine {
		t.Error("setup replaced a customized workflow after a no")
	}

	// An explicit yes takes the shipped copy.
	withStdin(t, "y\n", func() { syncWorkflows(root, askBeforeOverwrite) })
	got, _ := os.ReadFile(build)
	if string(got) == mine {
		t.Error("setup did not replace a customized workflow after a yes")
	}
	if !strings.Contains(string(got), "query_filter") {
		t.Errorf("reinstalled file is not the shipped copy: %q", got)
	}
}

// withStdin runs fn with os.Stdin replaced by a pipe carrying input, so
// confirmOverwrite is exercised on its real code path. Passing "" leaves
// the write end closed, which is what a non-interactive run sees.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if input != "" {
		if _, err := w.WriteString(input); err != nil {
			t.Fatal(err)
		}
	}
	w.Close()
	saved, savedReader := os.Stdin, promptIn
	os.Stdin, promptIn = r, nil
	defer func() { os.Stdin, promptIn = saved, savedReader }()
	fn()
}

func TestSyncPMAgentInstallsPrompt(t *testing.T) {
	root := t.TempDir()
	syncPMAgent(root, keepEdits)
	dest := filepath.Join(root, ".sortie", "pm-agent.md")
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("setup did not install the pm prompt: %v", err)
	}
	if !strings.Contains(string(raw), "united-into") {
		t.Error("installed pm prompt lacks the label contract it routes on")
	}
	// A hand-edited prompt survives setup without confirmation.
	const mine = "# mine\n"
	os.WriteFile(dest, []byte(mine), 0o644)
	withStdin(t, "", func() { syncPMAgent(root, askBeforeOverwrite) })
	if got, _ := os.ReadFile(dest); string(got) != mine {
		t.Error("setup replaced a customized pm prompt without confirmation")
	}
}

func TestEnsureGitignoreIgnoresSortieWholesale(t *testing.T) {
	root := t.TempDir()
	// Fresh repo: .gitignore is created with the wholesale entry only.
	ensureGitignore(root)
	raw, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(raw), ".sortie/\n") {
		t.Fatalf(".gitignore = %q, want wholesale .sortie/ entry", raw)
	}
	for _, e := range oldGitignoreEntries {
		if strings.Contains(string(raw), e+"\n") {
			t.Errorf(".gitignore still has granular entry %q", e)
		}
	}
	// Old granular scheme is migrated to wholesale on re-run.
	os.WriteFile(filepath.Join(root, ".gitignore"),
		[]byte("node_modules\n"+strings.Join(oldGitignoreEntries, "\n")+"\n"), 0o644)
	ensureGitignore(root)
	raw, _ = os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(raw), "node_modules\n") {
		t.Errorf("existing entries lost: %q", raw)
	}
	if !strings.Contains(string(raw), ".sortie/\n") {
		t.Errorf("no wholesale entry after migration: %q", raw)
	}
	// Selective tracking (negations) is left alone.
	os.WriteFile(filepath.Join(root, ".gitignore"),
		[]byte(".sortie/*\n!.sortie/config.yaml\n"), 0o644)
	ensureGitignore(root)
	raw, _ = os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Contains(string(raw), ".sortie/\n") {
		t.Errorf("selective gitignore overwritten: %q", raw)
	}
}

func TestLinkFileReplacesStaleDest(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "engine")
	os.WriteFile(target, []byte("x"), 0o755)
	dest := filepath.Join(root, "bin", "sortie")
	if err := linkFile(dest, target); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(dest); err != nil || got != target {
		t.Fatalf("readlink = %q, %v; want %q", got, err, target)
	}
	// Re-linking to a new target replaces the old symlink.
	newTarget := filepath.Join(root, "engine2")
	os.WriteFile(newTarget, []byte("y"), 0o755)
	if err := linkFile(dest, newTarget); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(dest); got != newTarget {
		t.Errorf("readlink = %q, want %q", got, newTarget)
	}
}

func TestDiscoverWorkflowsPicksUpNewFile(t *testing.T) {
	root := t.TempDir()
	syncWorkflows(root, keepEdits)
	// A hand-added workflow (no rebuild, no code change) is discovered.
	os.WriteFile(filepath.Join(root, ".sortie", "workflows", "WORKFLOW.triage.md"), []byte("---\n"), 0o644)
	// Non-workflow files are ignored.
	os.WriteFile(filepath.Join(root, ".sortie", "workflows", "notes.md"), []byte("x"), 0o644)
	got := discoverWorkflows(root)
	var names []string
	for _, l := range got {
		names = append(names, l.name)
	}
	want := []string{"build", "merge", "plan", "review-fix", "review", "triage"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("discoverWorkflows = %v, want %v", names, want)
	}
	if got[len(got)-1].file != "WORKFLOW.triage.md" {
		t.Errorf("triage file = %q", got[len(got)-1].file)
	}
}
