package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/kinged007/sortie-loop/internal/config"
)

// detectRepoFlag reads --repo=name from argv.
func detectRepoFlag() string {
	for i, a := range os.Args {
		if strings.HasPrefix(a, "--repo=") {
			return strings.TrimPrefix(a, "--repo=")
		}
		if a == "--repo" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// configRepo resolves the tracker repo for setup without requiring a token.
func configRepo(dir string) string {
	if v := detectRepoFlag(); v != "" {
		return v
	}
	if v := os.Getenv("SORTIE_LOOP_REPO"); v != "" {
		return v
	}
	if cfg, err := config.Load(dir); err == nil {
		return cfg.Repo
	}
	return guessRepo(dir)
}

// guessRepo derives owner/name from git, else empty (config keeps repo: blank).
func guessRepo(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return normalizeSlug(strings.TrimSpace(string(out)))
}

func normalizeSlug(s string) string {
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
