package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeRepo(t *testing.T) {
	cases := map[string]string{
		"https://github.com/octo/hello-world.git": "octo/hello-world",
		"https://github.com/octo/hello-world":     "octo/hello-world",
		"git@github.com:octo/hello-world.git":     "octo/hello-world",
		"octo/hello-world":                        "octo/hello-world",
		"owner/name/":                             "owner/name",
	}
	for in, want := range cases {
		if got := normalizeRepo(in); got != want {
			t.Errorf("normalizeRepo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadEnvAndMilestone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SORTIE_LOOP_REPO", "o/r")
	t.Setenv("SORTIE_LOOP_TOKEN", "tok")
	t.Setenv("SORTIE_LOOP_MILESTONE", "v2")
	t.Setenv("SORTIE_LOOP_ASSIGNEE", "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "o/r" || cfg.Token != "tok" || cfg.Tracker != "o/r" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.Filters["dev"] != `label:agent:quick,agent:build -label:agent:plan-needed milestone:"v2"` {
		t.Errorf("dev filter: %q", cfg.Filters["dev"])
	}
	if cfg.Filters["review"] != "" {
		t.Errorf("review filter should be empty, got %q", cfg.Filters["review"])
	}
}

func TestLoadAssigneeDefaultAndOptOut(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SORTIE_LOOP_REPO", "o/r")
	t.Setenv("SORTIE_LOOP_TOKEN", "tok")
	t.Setenv("SORTIE_LOOP_MILESTONE", "v2")
	os.Unsetenv("SORTIE_LOOP_ASSIGNEE")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Filters["plan"] != `label:agent:plan-needed milestone:"v2" assignee:@me` {
		t.Errorf("plan filter: %q", cfg.Filters["plan"])
	}
	if cfg.Filters["review"] != "assignee:@me" {
		t.Errorf("review filter: %q", cfg.Filters["review"])
	}
	t.Setenv("SORTIE_LOOP_ASSIGNEE", "")
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Filters["plan"] != `label:agent:plan-needed milestone:"v2"` {
		t.Errorf("opt-out plan filter: %q", cfg.Filters["plan"])
	}
	if cfg.Filters["review"] != "" {
		t.Errorf("opt-out review filter: %q", cfg.Filters["review"])
	}
}

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".sortie"), 0o755)
	os.WriteFile(filepath.Join(dir, ".sortie", "config.yaml"),
		[]byte("repo: file/o\ntoken: filetok\n"), 0o644)
	t.Setenv("SORTIE_LOOP_REPO", "")
	t.Setenv("SORTIE_LOOP_TOKEN", "")
	t.Setenv("SORTIE_LOOP_MILESTONE", "")
	t.Setenv("GH_MILESTONE", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "file/o" || cfg.Token != "filetok" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if cfg.CloneURL != "https://github.com/file/o.git" {
		t.Errorf("clone url: %q", cfg.CloneURL)
	}
}

func TestLoadNoRepoFails(t *testing.T) {
	dir := t.TempDir() // no git remote, no config
	t.Setenv("SORTIE_LOOP_REPO", "")
	if _, err := Load(dir); err == nil {
		t.Error("expected error with no repo detectable")
	}
}
