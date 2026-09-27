package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kinged007/sortie-loop/internal/config"
)

func main() {
	for _, a := range os.Args[1:] {
		if a == "--dump-version" {
			fmt.Println(sortieVersion)
			return
		}
	}
	if len(os.Args) > 1 && os.Args[1] == "setup" {
		dir := "."
		for i, a := range os.Args[2:] {
			_ = i
			if a == "--repo" || strings.HasPrefix(a, "--repo=") {
				continue
			}
			if !strings.HasPrefix(a, "-") {
				dir = a
			}
		}
		runSetup(dir)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help") {
		fmt.Println("Usage: sortie-loop [--no-server] [--no-dashboard] [--dashboard-port=N] [--unite] [repo-root]")
		fmt.Println("         sortie-loop setup [repo-root] [--repo=owner/name]")
		fmt.Println("  Run every WORKFLOW.*.md loop in .sortie/workflows/ against the repo at repo-root (default: cwd).")
		fmt.Println("  SORTIE_LOOP_ONLY=plan,build  start only these loops (default: all of them)")
		fmt.Println("  Settings live in <root>/.sortie/config.yaml; repo id defaults to the git remote.")
		fmt.Println("  Settings live in <root>/.sortie/config.yaml; repo id defaults to the git remote.")
		os.Exit(0)
	}
	dir := "."
	noServer := false
	noDashboard := false
	dashPort := 0
	unite := false
	for i, a := range os.Args[1:] {
		if a == "--repo" {
			_ = i
			continue
		}
		// skip the value following a bare --repo
		if i > 0 && os.Args[i] == "--repo" {
			continue
		}
		switch a {
		case "--no-server":
			noServer = true
		case "--no-dashboard":
			noDashboard = true
		case "--unite":
			unite = true
		default:
			if strings.HasPrefix(a, "--dashboard-port=") {
				if _, err := fmt.Sscanf(a, "--dashboard-port=%d", &dashPort); err != nil || dashPort < 0 {
					fatal(fmt.Errorf("invalid --dashboard-port=%q", a))
				}
				continue
			}
			if !strings.HasPrefix(a, "-") {
				dir = a
			}
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		fatal(err)
	}
	bin, err := resolveSortieBin()
	if err != nil {
		fatal(err)
	}
	envFile, err := writeEnvFile(abs, cfg)
	if err != nil {
		fatal(err)
	}
	// SORTIE_LOOP_ONLY selects a subset of the installed loops, for a
	// supervisor driving several repos. Unset starts every loop, so a
	// plain sortie-loop run is unchanged.
	loops, err := selectLoops(discoverWorkflows(abs), os.Getenv("SORTIE_LOOP_ONLY"))
	if err != nil {
		fatal(err)
	}
	if len(loops) == 0 {
		fatal(fmt.Errorf("no WORKFLOW.*.md files in %s", filepath.Join(abs, ".sortie", "workflows")))
	}
	for _, l := range loops {
		fmt.Printf("loop %-10s %s filter %q\n", l.name, l.file, cfg.FilterFor(l.name))
	}
	var lock *portLock
	if !noServer {
		// Serialize the port claim and child startup across sortie-loop
		// processes: two runs started together else probe before either
		// has bound, pick the same ports, and the later child loses the
		// bind (sortie exits when its HTTP server cannot bind).
		lock, err = lockStarts()
		if err != nil {
			fatal(fmt.Errorf("port lock: %w", err))
		}
	}
	// ponytail: one shared HTTP port per loop would collide; probe
	// upward from 7678 for a free port per loop (--no-server passes
	// --port 0 to disable loop servers entirely).
	loopPorts := make([]int, len(loops))
	if noServer {
		for i := range loopPorts {
			loopPorts[i] = 0
		}
	} else {
		var dash int
		// An explicit --dashboard-port=N always starts a dashboard on N
		// (fatal if busy). Otherwise a --unite run joins the live unite
		// dashboard when one answers, instead of starting a second one.
		if unite && dashPort == 0 && !noDashboard {
			if live := findUniteDashboard(); live > 0 {
				dashPort = -live
			}
		}
		loopPorts, dash, err = claimPorts(len(loops), dashPort, noDashboard)
		if err != nil {
			fatal(err)
		}
		if dashPort >= 0 {
			dashPort = dash
		}
	}
	var procs []*exec.Cmd
	defer func() {
		dropRegistry(abs)
		for _, p := range procs {
			if p.Process != nil {
				_ = p.Process.Signal(syscall.SIGTERM)
			}
		}
		for _, p := range procs {
			if p.Process != nil {
				_ = p.Wait()
			}
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		for _, p := range procs {
			if p.Process != nil {
				_ = p.Process.Signal(syscall.SIGTERM)
			}
		}
	}()
	for i, l := range loops {
		// The env override replaces the workflow's query_filter, so
		// defaultFilters must mirror each loop's label clauses (the
		// github-pr adapter enforces label: client-side); a filters:
		// entry narrows further, it does not add to the default.
		filter := cfg.FilterFor(l.name)
		env := append(os.Environ(), cfg.Env()...)
		if filter != "" {
			env = append(env, "SORTIE_TRACKER_QUERY_FILTER="+filter)
		}
		cmd, port, err := startLoopChild(bin, envFile, abs, l.file, env, loopPorts[i])
		if err != nil {
			fatal(fmt.Errorf("start %s loop: %w", l.name, err))
		}
		loopPorts[i] = port
		procs = append(procs, cmd)
	}
	// Startup done: every child bound its port, so the next run's probe
	// sees them; free the port-claim lock for the rest of the lifetime.
	if lock != nil {
		lock.release()
	}
	endpoints := make([]loopEndpoint, len(loops))
	for i, l := range loops {
		endpoints[i] = loopEndpoint{
			Repo:     abs,
			RepoName: repoName(abs),
			Loop:     l.name,
			Port:     loopPorts[i],
			DBPath:   dbPathFor(abs, l.file),
		}
	}
	if err := writeRegistry(abs, endpoints); err != nil {
		fmt.Fprintln(os.Stderr, "sortie-loop: registry:", err)
	}
	// dashPort is negative when this run joins a live unite dashboard:
	// its loops register above, and the existing page picks them up.
	if !noDashboard && dashPort > 0 {
		serveDashboard(dashPort, unite, endpoints)
	} else if dashPort < 0 {
		fmt.Printf("dashboard http://127.0.0.1:%d (unite: all repos)\n", -dashPort)
	}
	for _, p := range procs {
		go func(c *exec.Cmd) {
			if err := c.Wait(); err != nil {
				fmt.Fprintln(os.Stderr, "sortie-loop:", err)
			}
			_ = syscall.Kill(0, syscall.SIGTERM)
			os.Exit(1)
		}(p)
	}
	select {}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sortie-loop:", err)
	os.Exit(1)
}
