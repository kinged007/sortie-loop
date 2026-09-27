package main

import (
	"bytes"
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

func TestSelectLoops(t *testing.T) {
	all := []loopDef{{"build", "WORKFLOW.build.md"}, {"merge", "WORKFLOW.merge.md"}, {"plan", "WORKFLOW.plan.md"}}
	names := func(ls []loopDef) string {
		var n []string
		for _, l := range ls {
			n = append(n, l.name)
		}
		return strings.Join(n, ",")
	}
	// Unset/blank keeps every loop, in discovery order: a plain run is
	// unaffected by the selector.
	for _, only := range []string{"", "   "} {
		got, err := selectLoops(all, only)
		if err != nil || names(got) != "build,merge,plan" {
			t.Errorf("selectLoops(%q) = %v, %v; want all", only, names(got), err)
		}
	}
	// A subset keeps discovery order, not the order requested, and
	// carries the file through so the caller runs the right one.
	got, err := selectLoops(all, " merge , build ")
	if err != nil {
		t.Fatal(err)
	}
	if names(got) != "build,merge" || got[1].file != "WORKFLOW.merge.md" {
		t.Errorf("selectLoops = %v %q, want build,merge with merge's file", names(got), got[1].file)
	}
	// An unknown name is fatal even alongside known ones: starting the
	// rest would leave the asked-for loop's work unwatched.
	for _, only := range []string{"build,triage", "triage", "plan,build,review", ",", " , "} {
		if _, err := selectLoops(all, only); err == nil {
			t.Errorf("selectLoops(%q) = nil error, want an error", only)
		}
	}
	// Discovery order is preserved, not the requested order.
	if got, err := selectLoops(all, "plan,build"); err != nil || names(got) != "build,plan" {
		t.Errorf("selectLoops = %v, %v; want build,plan in discovery order", names(got), err)
	}
}

// TestSelectLoopsAgainstInstalledWorkflows pins the selector to the
// workflows setup actually installs, so a renamed or dropped loop is
// caught here rather than by a supervisor that names it.
func TestSelectLoopsAgainstInstalledWorkflows(t *testing.T) {
	root := t.TempDir()
	syncWorkflows(root, keepEdits)
	all := discoverWorkflows(root)
	if got, err := selectLoops(all, ""); err != nil || len(got) != len(all) {
		t.Fatalf("selectLoops(\"\") = %d loops, %v; want %d", len(got), err, len(all))
	}
	var want []string
	for _, l := range all {
		want = append(want, l.name)
	}
	got, err := selectLoops(all, strings.Join(want, ","))
	if err != nil {
		t.Fatalf("selectLoops(all names) = %v; every installed loop must be selectable", err)
	}
	if len(got) != len(all) {
		t.Errorf("selectLoops(all names) = %d loops, want %d", len(got), len(all))
	}
}

// resolveSortieBin finds the engine on PATH, which is the same
// ~/.local/bin directory sortie-loop itself lives in. ensureSortieLink
// then computed dest == bin and rewrote the engine as a symlink to
// itself, leaving a 29MB binary that can never run. The engine must
// survive being next to the loop binary.
func TestEnsureSortieLinkDoesNotDestroyTheEngineBesideIt(t *testing.T) {
	dir := t.TempDir()
	engine := filepath.Join(dir, "sortie")
	body := []byte("#!/bin/sh\necho engine\n")
	if err := os.WriteFile(engine, body, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real engine binary at the exact path ensureSortieLink would link.
	if err := os.WriteFile(filepath.Join(dir, "sortie"), body, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SORTIE_BIN", engine)
	t.Setenv("PATH", dir)

	ensureSortieLinkIn(dir)
	got, err := os.ReadFile(filepath.Join(dir, "sortie"))
	if err != nil {
		t.Fatalf("engine is unreadable: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("engine at %s was replaced: %q", dir, got)
	}
	info, err := os.Lstat(filepath.Join(dir, "sortie"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("engine became a symlink to itself")
	}
}

// A genuine second location still gets the convenience link, and an
// existing correct link is left alone rather than recreated.
func TestEnsureSortieLinkLinksADistinctEngine(t *testing.T) {
	dir := t.TempDir()
	loops := filepath.Join(dir, "bin")
	if err := os.MkdirAll(loops, 0o755); err != nil {
		t.Fatal(err)
	}
	engine := filepath.Join(dir, "elsewhere", "sortie")
	if err := os.MkdirAll(filepath.Dir(engine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(engine, []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SORTIE_BIN", engine)

	ensureSortieLinkIn(loops)
	link := filepath.Join(loops, "sortie")
	cur, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("no link created at %s: %v", link, err)
	}
	if cur != engine {
		t.Errorf("link = %q, want %q", cur, engine)
	}
	// Idempotent: a second call must not fail or change the link.
	ensureSortieLinkIn(loops)
	again, err := os.Readlink(link)
	if err != nil || again != engine {
		t.Errorf("second call changed the link: %q %v", again, err)
	}
}

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(a, link); err != nil {
		t.Fatal(err)
	}
	if !samePath(a, link) {
		t.Error("a and a symlink to it are the same file")
	}
	if !samePath(a, a) {
		t.Error("a path is the same as itself")
	}
	if samePath(a, filepath.Join(dir, "missing")) {
		t.Error("a missing path is the same as a")
	}
}
