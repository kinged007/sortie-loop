package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kinged007/sortie-loop/internal/config"
)

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

// ensureSortieBin returns the install.sh-downloaded sortie binary path.
// ponytail: checksum/signature verification when the release process
// matures beyond a single custom binary.
// ensureSortieLink exposes the resolved sortie engine as `sortie` next to
// the loop binary so bare `sortie` commands (e.g. `sortie validate`)
// work. A missing engine is a warning — config and labels don't need it.
func ensureSortieLink() {
	bin, err := resolveSortieBin()
	if err != nil {
		fmt.Println("warning: sortie engine not found; rerun install.sh or set SORTIE_BIN")
		return
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	dest := filepath.Join(filepath.Dir(self), "sortie")
	if cur, err := os.Readlink(dest); err == nil && cur == bin {
		return
	}
	if err := linkFile(dest, bin); err != nil {
		fmt.Println("warning:", err)
		return
	}
	fmt.Println("linked", dest, "->", bin)
}

// linkFile replaces dest with a symlink to target, creating the dir.
func linkFile(dest, target string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	os.Remove(dest)
	return os.Symlink(target, dest)
}

func ensureSortieBin() (string, error) {
	dest := filepath.Join(homeDir(), ".local", "share", "sortie-loop", "sortie-"+sortieVersion)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}
	return "", fmt.Errorf("sortie binary not found (looked for SORTIE_BIN, ./sortie, PATH, %s); download sortie "+sortieVersion+" from https://github.com/kinged007/sortie/releases and set SORTIE_BIN, or rerun install.sh", dest)
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
