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

	Dir       string // repo root (sortie dir is Dir/.sortie)
	SortieBin string // resolved sortie binary path
	EnvFile   string // path to generated env file passed via --env-file
	CloneURL  string // https clone URL derived from Repo
	Tracker   string // tracker project, always == Repo
	// Filters overrides the default per-loop query filters by loop name
	// (the WORKFLOW.*.md stem). Unknown loops get defaultFilter; known
	// loops fall back to their shipped default when unset.
	Filters map[string]string `yaml:"filters"`
	// Labels are the GitHub labels setup ensures exist (create-or-edit;
	// labels absent here are left alone). Empty = DefaultLabels.
	Labels     []Label `yaml:"labels"`
	Workspaces string  // .sortie/workspaces under Dir
}

// Label is one GitHub label managed by `sortie-loop setup`.
type Label struct {
	Name        string `yaml:"name"`
	Color       string `yaml:"color"`
	Description string `yaml:"description"`
}

// DefaultLabels are the labels for the shipped loops, seeded into a new
// config.yaml by setup. A custom loop adds its entries here, then setup
// creates them on GitHub (tracker support is GitHub-only).
var DefaultLabels = []Label{
	{"agent:quick", "fbca04", "Track: small change, merged to base, no PR"},
	{"agent:plan-needed", "d876e3", "Track: plan must be written and approved first"},
	{"agent:build", "1d76db", "Track: full development, lands via PR"},
	{"agent:pr-fix", "1d76db", "Trigger: PR feedback needs a fix build (alias of agent:build)"},
	{"backlog", "e4e669", "State: queued, not started"},
	{"in-progress", "1d76db", "State: work in progress"},
	{"review", "5319e7", "State: ready for human review"},
	{"done", "0e8a16", "State: completed"},
	{"needs-human", "d73a4a", "Escalation: agent needs a person"},
	{"agent:needs-review", "fbca04", "State: PR is waiting for agent review"},
	{"agent:reviewed", "5319e7", "State: agent review posted"},
	{"agent:review-complete", "0e8a16", "State: review feedback addressed"},
	{"agent:merge", "1d76db", "Command: agent should merge this PR"},
	{"agent:merged", "0e8a16", "State: agent merged the PR"},
}

// Load reads .sortie/config.yaml under dir (repo root), applies environment
// overrides, and derives git-based defaults. The config file must exist:
// a fresh checkout needs `sortie-loop setup` first, and running without
// it would silently fall back to defaults (wrong labels, no milestone)
// and dispatch on items the operator never scoped.
func Load(dir string) (*Config, error) {
	c := &Config{Dir: dir, Filters: map[string]string{}}
	if dir == "" {
		return nil, errors.New("config dir must not be empty")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".sortie", "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no .sortie/config.yaml in %s (run `sortie-loop setup` first)", dir)
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(raw) > 0 {
		var file struct {
			Repo      string            `yaml:"repo"`
			Token     string            `yaml:"token"`
			Milestone string            `yaml:"milestone"`
			Assignee  *string           `yaml:"assignee"`
			Filters   map[string]string `yaml:"filters"`
			Labels    []Label           `yaml:"labels"`
		}
		if err := yaml.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("parse .sortie/config.yaml: %w", err)
		}
		c.Repo, c.Token, c.Milestone = file.Repo, file.Token, file.Milestone
		c.Assignee = file.Assignee
		c.Filters = file.Filters
		c.Labels = file.Labels
	}
	if c.Filters == nil {
		c.Filters = map[string]string{}
	}
	if c.Labels == nil {
		c.Labels = DefaultLabels
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
	return c, nil
}

// defaultFilters are the label constraints for the shipped loops,
// mirroring each workflow's query_filter so the SORTIE_TRACKER_QUERY_FILTER
// env override (which replaces it) preserves the positive label clauses.
// The github-pr loops need them: the /pulls list path runs no search
// syntax, so the adapter enforces label: clauses client-side. A new
// WORKFLOW.*.md loop with no filters: entry gets defaultFilter (same
// scope, no label constraint), so it starts but matches nothing until
// the workflow's active_states or a filters: override narrows it.
var defaultFilters = map[string]string{
	"plan":       "label:agent:plan-needed -label:needs-human",
	"dev":        "label:agent:quick,agent:build -label:agent:plan-needed -label:needs-human",
	"review":     "label:agent:needs-review -label:needs-human",
	"review-fix": "label:agent:build,agent:pr-fix -label:needs-human",
	"merge":      "label:agent:merge -label:needs-human",
}

const defaultFilter = "-label:needs-human"

// FilterFor returns the query filter for a loop: the filters: override
// when set, else the shipped default for known loops, else defaultFilter.
// Milestone and assignee scope apply to every filter including overrides.
// Default assignee scope is @me (the token owner); empty = no restriction.
func (c *Config) FilterFor(name string) string {
	base := c.Filters[name]
	if base == "" {
		base = defaultFilters[name]
		if base == "" {
			base = defaultFilter
		}
	}
	return withScope(withMilestone(base, c.Milestone), c.assignee())
}

func (c *Config) assignee() string {
	if c.Assignee != nil {
		return *c.Assignee
	}
	if v, ok := os.LookupEnv("SORTIE_LOOP_ASSIGNEE"); ok {
		return v
	}
	return "@me"
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
