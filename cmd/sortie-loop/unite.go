package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	dashDefaultPort = 7677
	loopBasePort    = 7678
	probeCeilPort   = 7900
	probeStopEmpty  = 3
	registryDirName = "sortie-loop"
	registryName    = "registry.json"
)

// loopEndpoint describes one running sortie loop process: the repo it
// watches, the workflow it runs, where its dashboard API lives, and
// where its SQLite state lives for direct history reads.
type loopEndpoint struct {
	Repo     string `json:"repo"`
	RepoName string `json:"repo_name"`
	Loop     string `json:"loop"`
	Port     int    `json:"port"`
	DBPath   string `json:"db_path"`
}

// registry maps repo root -> its loop endpoints. One registry file is
// shared by every sortie-loop on the machine so a --unite dashboard can
// find them all. Entries carry a heartbeat; readers skip stale ones.
type registry struct {
	Entries map[string]registryEntry `json:"entries"`
}

type registryEntry struct {
	RepoName  string         `json:"repo_name"`
	UpdatedAt time.Time      `json:"updated_at"`
	PID       int            `json:"pid"`
	Loops     []loopEndpoint `json:"loops"`
}

// registryPath returns the shared registry file, creating its directory.
func registryPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, registryDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, registryName), nil
}

// repoName returns a short display name for a repo root: owner/name when
// the git remote parses, else the directory base.
func repoName(root string) string {
	if slug := guessRepo(root); strings.Contains(slug, "/") {
		return slug
	}
	return filepath.Base(root)
}

// probeFreePort reports whether nothing is listening on localhost:port.
func probeFreePort(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return true
	}
	_ = c.Close()
	return false
}

// claimPorts finds free ports for n loops plus the dashboard by
// probing upward from the base ports (busy ports are skipped — each
// loop server needs its own). Dashboard takes dashPort unless disabled.
// A --dashboard-port=N flag always claims N for this run (fatal if
// busy) so the operator can force a fresh dashboard; otherwise a live
// unite dashboard found by findUniteDashboard is reused (dash=-1).
// ponytail: two repos cannot share loop ports (one server per port),
// so each repo's loops stack to the right; the unite dashboard reads
// them all via the registry instead.
func claimPorts(n int, dashPort int, noDashboard bool) (loops []int, dash int, err error) {
	loops = make([]int, 0, n)
	for p := loopBasePort; len(loops) < n && p <= probeCeilPort; p++ {
		if probeFreePort(p) {
			loops = append(loops, p)
		}
	}
	if len(loops) < n {
		return nil, 0, fmt.Errorf("only %d free ports below %d, need %d", len(loops), probeCeilPort, n)
	}
	if noDashboard {
		return loops, 0, nil
	}
	// Negative dashPort joins a live dashboard found earlier: no port
	// is claimed and none is started (main prints the reused URL).
	if dashPort < 0 {
		return loops, dashPort, nil
	}
	if dashPort == 0 {
		dashPort = dashDefaultPort
	}
	if probeFreePort(dashPort) {
		return loops, dashPort, nil
	}
	return nil, 0, fmt.Errorf("dashboard port %d is busy (another dashboard is running?)", dashPort)
}

// findUniteDashboard returns the port of a live unite dashboard: the
// first busy port at or below the dashboard default whose page marks
// itself unite. Loop API ports and non-unite dashboards are skipped.
func findUniteDashboard() int {
	for p := dashDefaultPort; p >= loopBasePort-8 && p > 0; p-- {
		if found := findUniteDashboardOn(p); found > 0 {
			return found
		}
	}
	return 0
}

// findUniteDashboardOn reports port when its page marks itself unite;
// split out so tests can target ephemeral ports.
func findUniteDashboardOn(p int) int {
	client := &http.Client{Timeout: 2 * time.Second}
	if probeFreePort(p) {
		return 0
	}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", p))
	if err != nil {
		return 0
	}
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if rerr != nil || resp.StatusCode != http.StatusOK {
		return 0
	}
	if strings.Contains(string(body), "\u2014 Unite") {
		return p
	}
	return 0
}

// writeRegistry records this sortie-loop's endpoints so a --unite
// dashboard started later can discover them without probing.
func writeRegistry(root string, endpoints []loopEndpoint) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	reg := registry{Entries: map[string]registryEntry{}}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &reg)
		if reg.Entries == nil {
			reg.Entries = map[string]registryEntry{}
		}
	}
	reg.Entries[root] = registryEntry{
		RepoName:  repoName(root),
		UpdatedAt: time.Now().UTC(),
		PID:       os.Getpid(),
		Loops:     endpoints,
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// dropRegistry removes this repo's entry on clean shutdown.
func dropRegistry(root string) {
	path, err := registryPath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil || reg.Entries == nil {
		return
	}
	delete(reg.Entries, root)
	data, err = json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

// dbPathFor resolves a workflow's db_path front-matter the way sortie
// does: relative paths join with the workflow directory (the repo root
// here, since sortie runs with Dir=root). Falls back to .sortie.db.
func dbPathFor(root, file string) string {
	data, err := os.ReadFile(filepath.Join(root, ".sortie", "workflows", file))
	if err != nil {
		return filepath.Join(root, ".sortie", "workflows", ".sortie.db")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "db_path:"); ok {
			v = strings.TrimSpace(v)
			if v == "" {
				break
			}
			if filepath.IsAbs(v) {
				return v
			}
			return filepath.Join(root, ".sortie", "workflows", v)
		}
		if strings.TrimSpace(line) == "---" && !strings.HasPrefix(strings.TrimSpace(line), "db_path") {
			continue
		}
	}
	return filepath.Join(root, ".sortie", "workflows", ".sortie.db")
}

// readRegistry returns live endpoints across all repos: entries whose
// loop API ports still answer, probed upward from 7678. After 3
// consecutive empty ports the scan stops, tolerating gaps left by
// stopped loops in between. Stale registry entries (dead owner, old
// heartbeat) are skipped, never deleted here.
func readRegistry() []loopEndpoint {
	path, err := registryPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var reg registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil
	}
	known := map[int]loopEndpoint{}
	for _, e := range reg.Entries {
		if time.Since(e.UpdatedAt) > 5*time.Minute && !pidAlive(e.PID) {
			continue
		}
		for _, l := range e.Loops {
			if l.Port > 0 {
				known[l.Port] = l
			}
		}
	}
	var out []loopEndpoint
	empty := 0
	for p := loopBasePort; p <= probeCeilPort && empty < probeStopEmpty; p++ {
		if probeFreePort(p) {
			empty++
			continue
		}
		empty = 0
		if l, ok := known[p]; ok {
			out = append(out, l)
			continue
		}
		// ponytail: answering port with no registry entry is a
		// loop whose owner died without cleanup; include it
		// under an unknown repo so its activity stays visible.
		out = append(out, loopEndpoint{Repo: "unknown", RepoName: "unknown", Loop: "unknown", Port: p})
	}
	return out
}

// pidAlive reports whether pid names a running process.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 performs error checking without delivery; on unix a nil
	// error means the process exists (or we lack permission, in which
	// case treating it as alive is the safe side for discovery).
	return p.Signal(syscall.Signal(0)) == nil
}
