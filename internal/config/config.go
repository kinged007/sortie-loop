// Package config resolves the loop's runtime settings from .sortie/config.yaml
// with environment overrides, and derives the repo identity from git.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the resolved runtime settings for one loop run.
type Config struct {
	Repo      string `yaml:"repo"`
	Token     string `yaml:"token"`
	Milestone string `yaml:"milestone"`
	// Assignee restricts every loop to items assigned to this user.
	// Empty = no restriction. Unset = @me (the token owner).
	Assignee *string `yaml:"assignee"`

	Dir        string            // repo root (sortie dir is Dir/.sortie)
	SortieBin  string            // resolved sortie binary path
	EnvFile    string            // path to generated env file passed via --env-file
	CloneURL   string            // https clone URL derived from Repo
	Tracker    string            // tracker project, always == Repo
	Filters    map[string]string // per-loop query filters with milestone applied
	Workspaces string            // .sortie/workspaces under Dir
}

// Load reads .sortie/config.yaml under dir (repo root), applies environment
// overrides, and derives git-based defaults. Missing config file is fine:
// every field has a default or environment fallback.
func Load(dir string) (*Config, error) {
	c := &Config{Dir: dir, Filters: map[string]string{}}
	if dir == "" {
		return nil, errors.New("config dir must not be empty")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".sortie", "config.yaml"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read config: %w", err)
	}
	// Default: only pick up items assigned to the token owner.
	assignee := "@me"
	if len(raw) > 0 {
		var file struct {
			Repo      string  `yaml:"repo"`
			Token     string  `yaml:"token"`
			Milestone string  `yaml:"milestone"`
			Assignee  *string `yaml:"assignee"`
		}
		if err := yaml.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("parse .sortie/config.yaml: %w", err)
		}
		c.Repo, c.Token, c.Milestone = file.Repo, file.Token, file.Milestone
		if file.Assignee != nil {
			assignee = *file.Assignee
		}
		if v, ok := os.LookupEnv("SORTIE_LOOP_ASSIGNEE"); ok {
			assignee = v
		}
		c.Assignee = file.Assignee
	}
	if v, ok := os.LookupEnv("SORTIE_LOOP_ASSIGNEE"); ok {
		assignee = v
	}
	if v := os.Getenv("SORTIE_LOOP_REPO"); v != "" {
		c.Repo = v
	}
	if v := os.Getenv("SORTIE_LOOP_TOKEN"); v != "" {
		c.Token = v
	}
	if v := os.Getenv("SORTIE_LOOP_MILESTONE"); v != "" {
		c.Milestone = v
	}
	if v := os.Getenv("GH_MILESTONE"); v != "" && c.Milestone == "" {
		c.Milestone = v
	}
	if c.Repo == "" {
		slug, err := detectRepo(dir)
		if err != nil {
			return nil, err
		}
		c.Repo = slug
	}
	c.Repo = normalizeRepo(c.Repo)
	if c.Repo == "" || !strings.Contains(c.Repo, "/") {
		return nil, fmt.Errorf("expected owner/name, got %q (set repo: in .sortie/config.yaml or SORTIE_LOOP_REPO)", c.Repo)
	}
	if c.Token == "" {
		if v := os.Getenv("GITHUB_TOKEN"); v != "" {
			c.Token = v
		} else if v := os.Getenv("GH_TOKEN"); v != "" {
			c.Token = v
		}
	}
	c.Tracker = c.Repo
	c.CloneURL = "https://github.com/" + c.Repo + ".git"
	c.Workspaces = filepath.Join(dir, ".sortie", "workspaces")
	c.Filters = map[string]string{
		"plan":       withScope(withMilestone("label:agent:plan-needed -label:needs-human", c.Milestone), assignee),
		"dev":        withScope(withMilestone("label:agent:quick,agent:build -label:agent:plan-needed -label:needs-human", c.Milestone), assignee),
		"review":     withScope("-label:needs-human", assignee),
		"review-fix": withScope("-label:needs-human", assignee),
		"merge":      withScope("-label:needs-human", assignee),
	}
	return c, nil
}

// Env returns the SORTIE_* environment for one sortie subprocess.
// The token travels as SORTIE_TRACKER_API_KEY so it overrides
// tracker.api_key through sortie's curated env layer.
func (c *Config) Env() []string {
	return []string{
		"SORTIE_TRACKER_PROJECT=" + c.Tracker,
		"SORTIE_TRACKER_API_KEY=" + c.Token,
		"SORTIE_LOOP_CLONE_URL=" + c.CloneURL,
		"SORTIE_LOOP_WORKSPACES=" + c.Workspaces,
	}
}

func withScope(base, assignee string) string {
	if assignee == "" {
		return base
	}
	if base == "" {
		return "assignee:" + assignee
	}
	return base + " assignee:" + assignee
}

func withMilestone(base, milestone string) string {
	if milestone == "" {
		return base
	}
	if base == "" {
		return `milestone:"` + milestone + `"`
	}
	return base + ` milestone:"` + milestone + `"`
}

// detectRepo derives owner/name from the git remote in dir.
func detectRepo(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return "", errors.New("cannot detect repo from git remote (not a git checkout or no origin remote? set repo: in .sortie/config.yaml or SORTIE_LOOP_REPO)")
	}
	return normalizeRepo(strings.TrimSpace(string(out))), nil
}

func normalizeRepo(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	if i := strings.Index(s, "github.com"); i >= 0 {
		s = s[i+len("github.com"):]
	}
	s = strings.Trim(s, "/:")
	if strings.HasPrefix(s, "git@") {
		s = strings.TrimPrefix(s, "git@")
	}
	if strings.Count(s, "/") > 1 {
		parts := strings.Split(s, "/")
		s = strings.Join(parts[len(parts)-2:], "/")
	}
	return s
}
