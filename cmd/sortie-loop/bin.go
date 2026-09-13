package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kinged007/sortie-loop/internal/config"
)

// workflowPath returns the embedded workflow file for name,
// materialized next to the binary on first use.
func workflowPath(name string) string {
	return mustMaterialize(name)
}

// resolveSortieBin finds the sortie binary: SORTIE_BIN env, beside the
// loop binary, on PATH, else the install.sh symlink under PREFIX.
func resolveSortieBin() (string, error) {
	if v := os.Getenv("SORTIE_BIN"); v != "" {
		return v, nil
	}
	if self, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(self), "sortie"); p != self {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	if p, err := exec.LookPath("sortie"); err == nil {
		return p, nil
	}
	return ensureSortieBin()
}

// writeEnvFile writes the resolved settings to .sortie/.env.loop so the
// workflows can load them via --env-file without touching the user's env.
func writeEnvFile(dir string, cfg *config.Config) (string, error) {
	path := filepath.Join(dir, ".sortie", ".env.loop")
	body := "SORTIE_TRACKER_PROJECT=" + cfg.Tracker + "\n" +
		"SORTIE_LOOP_CLONE_URL=" + cfg.CloneURL + "\n" +
		"SORTIE_LOOP_WORKSPACES=" + cfg.Workspaces + "\n" +
		"SORTIE_TRACKER_API_KEY=" + cfg.Token + "\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", err
	}
	cfg.EnvFile = path
	return path, nil
}

// ensureSortieBin returns the install.sh-managed sortie binary path.
// ponytail: no release ships pi/github-pr yet, so there is nothing to
// download; recheck when the first supporting release lands.
func ensureSortieBin() (string, error) {
	dest := filepath.Join(homeDir(), ".local", "share", "sortie-loop", "sortie-"+sortieVersion)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	return "", fmt.Errorf("sortie binary not found (looked for SORTIE_BIN, ./sortie, PATH, %s); install sortie "+sortieVersion+" and set SORTIE_BIN", dest)
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
