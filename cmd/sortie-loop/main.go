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
		fmt.Println("Usage: sortie-loop [--no-server] [repo-root]")
		fmt.Println("         sortie-loop setup [repo-root] [--repo=owner/name]")
		fmt.Println("  Run plan, dev, and review loops against the repo at repo-root (default: cwd).")
		fmt.Println("  Settings live in <root>/.sortie/config.yaml; repo id defaults to the git remote.")
		fmt.Println("  Settings live in <root>/.sortie/config.yaml; repo id defaults to the git remote.")
		os.Exit(0)
	}
	dir := "."
	noServer := false
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
		default:
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
	loops := []struct{ name, file, filter string }{
		{"plan", "WORKFLOW.plan.md", cfg.Filters["plan"]},
		{"dev", "WORKFLOW.dev.md", cfg.Filters["dev"]},
		{"review", "WORKFLOW.review.md", cfg.Filters["review"]},
	}
	// ponytail: one shared HTTP port across loops would collide; give each
	// loop its own (--no-server passes --port 0 to disable entirely).
	ports := []string{"7678", "7679", "7680"}
	if noServer {
		ports = []string{"0", "0", "0"}
	}
	var procs []*exec.Cmd
	defer func() {
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
		env := append(os.Environ(), cfg.Env()...)
		if l.filter != "" {
			env = append(env, "SORTIE_TRACKER_QUERY_FILTER="+l.filter)
		}
		cmd := exec.Command(bin, "--env-file", envFile, "--port", ports[i], workflowPath(l.file))
		cmd.Dir = abs
		cmd.Env = env
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fatal(fmt.Errorf("start %s loop: %w", l.name, err))
		}
		procs = append(procs, cmd)
	}
	for _, p := range procs {
		_ = p.Wait()
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sortie-loop:", err)
	os.Exit(1)
}
