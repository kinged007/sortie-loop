package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// runRemote asks the sortie-loop already serving this repo-root to stop
// or restart.
//
// The running process is the only thing that can decide when a loop
// ends, because it is the only thing watching it, so the request travels
// as a signal rather than as work done here. This command is a client of
// that: it finds the process through the shared registry and signals it.
// It is what lets a supervisor say "stop this repo, but not mid-run"
// without knowing anything about the loops inside it.
func runRemote(action string, args []string) {
	dir := "."
	whenIdle := false
	for i, a := range args {
		switch {
		case a == "--when-idle":
			whenIdle = true
		case a == "--repo":
			continue
		case i > 0 && args[i-1] == "--repo":
			continue
		case strings.HasPrefix(a, "-"):
			fatal(fmt.Errorf("unknown flag %q", a))
		default:
			dir = a
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal(err)
	}
	pid, err := runningPID(abs)
	if err != nil {
		fatal(err)
	}
	// restart without --when-idle is not a signal: it is a stop followed
	// by a fresh process, which the caller starts. Signalling would keep
	// a run that is meant to be replaced.
	sig := sigRestartIdle
	switch {
	case action == "stop" && whenIdle:
		sig = sigStopIdle
	case action == "stop":
		sig = syscall.SIGTERM
	}
	if action == "restart" && !whenIdle {
		fatal(fmt.Errorf("restart without --when-idle is not signalled: stop this run and start a new one"))
	}
	if err := syscall.Kill(pid, sig); err != nil {
		fatal(fmt.Errorf("signal %s: %w", sig, err))
	}
	what := "restarting each loop when it goes idle"
	switch sig {
	case sigStopIdle:
		what = "stopping each loop when it goes idle"
	case syscall.SIGTERM:
		what = "stopped now"
	}
	fmt.Printf("sortie-loop: %s (pid %d) %s\n", abs, pid, what)
}

// runningPID returns the pid of the live sortie-loop serving root, or an
// error when the registry has no live entry for it. A stale entry is
// worse than none: a repo whose run died leaves its row behind, and
// signalling that pid would land on an unrelated process.
func runningPID(root string) (int, error) {
	path, err := registryPath()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("no sortie-loop is running for %s: %w", root, err)
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return 0, fmt.Errorf("registry: %w", err)
	}
	e, ok := reg.Entries[root]
	if !ok {
		return 0, fmt.Errorf("no sortie-loop is running for %s", root)
	}
	if e.PID <= 0 || syscall.Kill(e.PID, 0) != nil {
		return 0, fmt.Errorf("the sortie-loop registered for %s is no longer running", root)
	}
	return e.PID, nil
}
