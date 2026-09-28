package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

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
	self, err := os.Executable()
	if err != nil {
		return
	}
	ensureSortieLinkIn(filepath.Dir(self))
}

// ensureSortieLinkIn is ensureSortieLink for a chosen directory, so the
// self-link guard is testable without relocating the running binary.
func ensureSortieLinkIn(dir string) {
	bin, err := resolveSortieBin()
	if err != nil {
		fmt.Println("warning: sortie engine not found; rerun install.sh or set SORTIE_BIN")
		return
	}
	dest := filepath.Join(dir, "sortie")
	if samePath(dest, bin) {
		// The engine resolveSortieBin found is already this path, so
		// linking would point the name at itself and destroy the binary.
		// Reached whenever both are installed in one directory, which is
		// the normal case.
		return
	}
	if cur, err := os.Readlink(dest); err == nil && cur == bin {
		return
	}
	if err := linkFile(dest, bin); err != nil {
		fmt.Println("warning:", err)
		return
	}
	fmt.Println("linked", dest, "->", bin)
}

// samePath reports whether two paths name the same file, following
// symlinks. A path that cannot be stat'd is not the same as anything.
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	sa, err := os.Stat(a)
	if err != nil {
		return false
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

// linkFile replaces dest with a symlink to target, creating the dir.
func linkFile(dest, target string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	os.Remove(dest)
	return os.Symlink(target, dest)
}

// sortieShareDirs are the directories the installer may have put the
// version-named engine in.
//
// install.sh honours --prefix and writes the engine to
// <prefix>/share/sortie-loop, so a custom prefix is where the engine
// actually is. Looking only at ~/.local meant resolveSortieBin failed
// for every non-default prefix, which left ensureSortieLink unable to
// create the sortie symlink and reported "engine not found" on a
// perfectly complete installation. Deriving the directory from the
// running binary makes --prefix work, with the home default kept for an
// install whose binaries moved later.
func sortieShareDirs() []string {
	var dirs []string
	add := func(dir string) {
		if dir != "" && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	if self, err := os.Executable(); err == nil {
		abs, err := filepath.Abs(self)
		if err == nil {
			// <prefix>/bin/sortie-loop -> <prefix>/share/sortie-loop
			add(filepath.Join(filepath.Dir(filepath.Dir(abs)), "share", "sortie-loop"))
		}
	}
	if home := homeDir(); home != "" {
		add(filepath.Join(home, ".local", "share", "sortie-loop"))
	}
	return dirs
}

func ensureSortieBin() (string, error) {
	return ensureSortieBinIn(sortieShareDirs())
}

// ensureSortieBinIn is ensureSortieBin over an explicit candidate list, so
// the search order is testable without relocating the running binary.
func ensureSortieBinIn(dirs []string) (string, error) {
	for _, dir := range dirs {
		dest := filepath.Join(dir, "sortie-"+sortieVersion)
		if _, err := os.Stat(dest); err == nil {
			return dest, nil
		}
	}
	if len(dirs) == 0 {
		return "", fmt.Errorf("sortie binary not found (looked for SORTIE_BIN, ./sortie, PATH); download sortie " + sortieVersion + " from https://github.com/kinged007/sortie/releases and set SORTIE_BIN, or rerun install.sh")
	}
	return "", fmt.Errorf("sortie binary not found (looked for SORTIE_BIN, ./sortie, PATH, and %s); download sortie "+sortieVersion+" from https://github.com/kinged007/sortie/releases and set SORTIE_BIN, or rerun install.sh", strings.Join(dirs, ", ")+"/sortie-"+sortieVersion)
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}
