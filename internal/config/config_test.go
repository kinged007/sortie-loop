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
	if cfg.FilterFor("dev") != `label:agent:quick,agent:build -label:agent:plan-needed -label:needs-human milestone:"v2"` {
		t.Errorf("dev filter: %q", cfg.FilterFor("dev"))
	}
	if cfg.FilterFor("review") != `label:agent:needs-review -label:needs-human milestone:"v2"` {
		t.Errorf("review filter should exclude needs-human, got %q", cfg.FilterFor("review"))
	}
	if cfg.FilterFor("merge") != `label:agent:merge -label:needs-human milestone:"v2"` {
		t.Errorf("merge filter should exclude needs-human, got %q", cfg.FilterFor("merge"))
	}
	if cfg.FilterFor("review-fix") != `label:agent:build -label:needs-human milestone:"v2"` {
		t.Errorf("review-fix filter should exclude needs-human, got %q", cfg.FilterFor("review-fix"))
	}
	// Unknown loops get the default filter (no label constraint).
	if cfg.FilterFor("triage") != `-label:needs-human milestone:"v2"` {
		t.Errorf("unknown loop filter: %q", cfg.FilterFor("triage"))
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
	if cfg.FilterFor("plan") != `label:agent:plan-needed -label:needs-human milestone:"v2" assignee:@me` {
		t.Errorf("plan filter: %q", cfg.FilterFor("plan"))
	}
	if cfg.FilterFor("review") != `label:agent:needs-review -label:needs-human milestone:"v2" assignee:@me` {
		t.Errorf("review filter: %q", cfg.FilterFor("review"))
	}
	if cfg.FilterFor("merge") != `label:agent:merge -label:needs-human milestone:"v2" assignee:@me` {
		t.Errorf("merge filter: %q", cfg.FilterFor("merge"))
	}
	if cfg.FilterFor("review-fix") != `label:agent:build -label:needs-human milestone:"v2" assignee:@me` {
		t.Errorf("review-fix filter: %q", cfg.FilterFor("review-fix"))
	}
	t.Setenv("SORTIE_LOOP_ASSIGNEE", "")
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FilterFor("plan") != `label:agent:plan-needed -label:needs-human milestone:"v2"` {
		t.Errorf("opt-out plan filter: %q", cfg.FilterFor("plan"))
	}
	if cfg.FilterFor("review") != `label:agent:needs-review -label:needs-human milestone:"v2"` {
		t.Errorf("opt-out review filter: %q", cfg.FilterFor("review"))
	}
	if cfg.FilterFor("merge") != `label:agent:merge -label:needs-human milestone:"v2"` {
		t.Errorf("opt-out merge filter: %q", cfg.FilterFor("merge"))
	}
	if cfg.FilterFor("review-fix") != `label:agent:build -label:needs-human milestone:"v2"` {
		t.Errorf("opt-out review-fix filter: %q", cfg.FilterFor("review-fix"))
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

func TestLoadFilterOverrides(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".sortie"), 0o755)
	os.WriteFile(filepath.Join(dir, ".sortie", "config.yaml"),
		[]byte("repo: file/o\ntoken: filetok\nfilters:\n  triage: label:agent:triage\n  dev: label:agent:custom\n"), 0o644)
	t.Setenv("SORTIE_LOOP_REPO", "")
	t.Setenv("SORTIE_LOOP_TOKEN", "")
	t.Setenv("SORTIE_LOOP_MILESTONE", "")
	t.Setenv("GH_MILESTONE", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("SORTIE_LOOP_ASSIGNEE", "")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FilterFor("triage") != "label:agent:triage" {
		t.Errorf("override filter: %q", cfg.FilterFor("triage"))
	}
	if cfg.FilterFor("dev") != "label:agent:custom" {
		t.Errorf("known-loop override: %q", cfg.FilterFor("dev"))
	}
	if cfg.FilterFor("merge") != "label:agent:merge -label:needs-human" {
		t.Errorf("untouched default: %q", cfg.FilterFor("merge"))
	}
}

func TestLoadNoRepoFails(t *testing.T) {
	dir := t.TempDir() // no git remote, no config
	t.Setenv("SORTIE_LOOP_REPO", "")
	if _, err := Load(dir); err == nil {
		t.Error("expected error with no repo detectable")
	}
}
