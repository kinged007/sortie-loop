package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/kinged007/sortie-loop/internal/config"
)

// cmdline is the parsed command line.
type cmdline struct {
	setup       bool
	repo        string
	dir         string
	noServer    bool
	noDashboard bool
	unite       bool
	dashPort    int
	dumpVersion bool
	showVersion bool
}

// errHelp is returned by parseArgs when usage was asked for.
var errHelp = errors.New("help")

// parseArgs reads the command line. A flag that takes a value accepts it
// as the next argument or after "=", and the value is consumed either
// way, so a repository slug is never mistaken for the directory. The
// directory is the last bare argument and may sit anywhere, so
// `sortie-loop setup ./repo --repo owner/name` and
// `sortie-loop setup --repo owner/name ./repo` both target ./repo.
func parseArgs(argv []string) (cmdline, error) {
	var c cmdline
	var bare []string
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		name, inline, hasInline := strings.Cut(arg, "=")
		value := func() (string, error) {
			if hasInline {
				return inline, nil
			}
			if i+1 >= len(argv) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return argv[i], nil
		}
		switch name {
		case "setup":
			c.setup = true
		case "--dump-version":
			c.dumpVersion = true
		case "--version", "-V":
			c.showVersion = true
		case "help", "-h", "--help":
			return c, errHelp
		case "--repo":
			v, err := value()
			if err != nil {
				return c, err
			}
			c.repo = v
		case "--no-server":
			c.noServer = true
		case "--no-dashboard":
			c.noDashboard = true
		case "--unite":
			c.unite = true
		case "--dashboard-port":
			v, err := value()
			if err != nil {
				return c, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return c, fmt.Errorf("invalid --dashboard-port=%q", v)
			}
			c.dashPort = n
		default:
			if strings.HasPrefix(arg, "-") {
				return c, fmt.Errorf("unknown flag %s", name)
			}
			bare = append(bare, arg)
		}
	}
	if len(bare) > 0 {
		c.dir = bare[len(bare)-1]
	}
	return c, nil
}

// usageWriter receives the usage text; a variable so a test can read it.
var usageWriter = func(s string) { fmt.Println(s) }

func usage() {
	usageWriter("Usage: sortie-loop [--no-server] [--no-dashboard] [--dashboard-port=N] [--unite] [repo-root]\n" +
		"       sortie-loop setup [repo-root] [--repo=owner/name]\n" +
		"  Run every WORKFLOW.*.md loop in .sortie/workflows/ against the repo at repo-root (default: cwd).\n" +
		"  Settings live in <root>/.sortie/config.yaml; repo id defaults to the git remote.\n" +
		"\n" +
		"  --repo owner/name  tracker repository (default: the origin remote)\n" +
		"  --unite            join or start the shared dashboard across repos\n" +
		"  --dashboard-port=N serve the dashboard on port N\n" +
		"  --no-dashboard     do not serve the dashboard\n" +
		"  --no-server        do not start the per-loop debug servers\n" +
		"  --version          print the sortie-loop version\n" +
		"  --dump-version     print the pinned engine version\n")
}

func main() { os.Exit(run()) }

func run() (code int) {
	// fatal reports through a panic so the deferred cleanup above runs on
	// every error path, including a loop that failed to start after
	// earlier loops were already running.
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fe, ok := r.(fatalError)
		if !ok {
			panic(r)
		}
		fmt.Fprintln(os.Stderr, "sortie-loop:", fe.err)
		code = 1
	}()
	c, err := parseArgs(os.Args[1:])
	if errors.Is(err, errHelp) {
		usage()
		return 0
	}
	if err != nil {
		fatal(err)
	}
	if c.dumpVersion {
		fmt.Println(engineVersion())
		return
	}
	if c.showVersion {
		fmt.Printf("sortie-loop %s (engine %s)\n", version, engineVersion())
		return
	}
	if c.setup {
		runSetup(c.dir, c.repo)
		return
	}
	dir := c.dir
	if dir == "" {
		dir = "."
	}
	noServer, noDashboard, dashPort, unite := c.noServer, c.noDashboard, c.dashPort, c.unite
	abs, err := filepath.Abs(dir)
	if err != nil {
		fatal(err)
	}
	cfg, err := config.Load(abs)
	if err != nil {
		fatal(err)
	}
	// Checked before anything is started: an unauthenticated run would
	// otherwise claim an issue and start an agent before the engine
	// reported the failure.
	if err := cfg.CheckToken(); err != nil {
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
	loops := discoverWorkflows(abs)
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
	// A loop child exiting ends the run, exactly as a signal does: the
	// deferred cleanup stops the remaining children and drops the
	// registry entry, then run returns the exit code. Signalling the
	// children directly rather than process group 0 keeps the signal away
	// from the shell and every other process sharing the terminal.
	exited := make(chan error, len(procs))
	for _, p := range procs {
		go func(c *exec.Cmd) {
			if err := c.Wait(); err != nil {
				exited <- fmt.Errorf("loop process: %w", err)
				return
			}
			exited <- nil
		}(p)
	}
	select {
	case s := <-sig:
		fmt.Fprintf(os.Stderr, "sortie-loop: %s, stopping loops\n", s)
	case err := <-exited:
		if err != nil {
			return 1
		}
	}
	return 0
}

// fatalError carries a message out to run's recover, so the deferred
// cleanup still runs before the process exits. Calling os.Exit from here
// would skip it and leave loop children running.
type fatalError struct{ err error }

func fatal(err error) { panic(fatalError{err}) }
