package main

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// tableExists reports whether the DB has the named table, so fresh loop DBs
// whose migrations have not checkpointed yet read as empty, not errors.
func tableExists(db *sql.DB, name string) bool {
	var found string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&found); err != nil {
		return false
	}
	return true
}

// loopStats holds one loop's figures: live state from its /api/v1/state
// plus all-time history read directly from its SQLite DB, so totals
// survive loop restarts (the upstream dashboard reseeds from the same
// aggregate_metrics row but serves process memory, which starts at 0).
type loopStats struct {
	Endpoint loopEndpoint `json:"endpoint"`
	Live     bool         `json:"live"`
	// Live figures from /api/v1/state.
	Running         []runRow `json:"running"`
	Retrying        int      `json:"retrying"`
	BudgetExhausted int      `json:"budget_exhausted"`
	SlotsFree       int      `json:"slots_free"`
	// All-time figures from the DB.
	AllInput   int64   `json:"all_input"`
	AllOutput  int64   `json:"all_output"`
	AllTotal   int64   `json:"all_total"`
	AllCache   int64   `json:"all_cache"`
	Runs       int     `json:"runs"`
	Succeeded  int     `json:"succeeded"`
	Failed     int     `json:"failed"`
	RuntimeSec float64 `json:"runtime_sec"`
	DBLive     bool    `json:"db_live"`
	DBError    string  `json:"db_error,omitempty"`
}

type runRow struct {
	Identifier string `json:"identifier"`
	State      string `json:"state"`
	Turns      int    `json:"turns"`
	Tokens     int64  `json:"tokens"`
	Model      string `json:"model"`
	StartedAt  string `json:"started_at"`
}

type issueStats struct {
	Identifier string `json:"identifier"`
	RepoName   string `json:"repo_name"`
	Loop       string `json:"loop"`
	Runs       int    `json:"runs"`
	Succeeded  int    `json:"succeeded"`
	Failed     int    `json:"failed"`
	Tokens     int64  `json:"tokens"`
	Input      int64  `json:"input"`
	Output     int64  `json:"output"`
	CacheRead  int64  `json:"cache_read"`
	LastStatus string `json:"last_status"`
	LastRun    string `json:"last_run"`
}

// openRO opens a sortie SQLite DB read-only without disturbing the
// writer. Plain mode=ro (not immutable=1): sortie runs in WAL mode and
// fresh tables/rows sit in the -wal sidecar until checkpoint, which an
// immutable open cannot see. A read-only query takes no write lock, and
// _query_only blocks writes through this handle.
func openRO(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_query_only=1")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// readLoopDB fills the all-time figures from aggregate_metrics +
// run_history. Missing tables (older DBs) leave zeros, not errors.
func readLoopDB(st *loopStats) {
	db, err := openRO(st.Endpoint.DBPath)
	if err != nil {
		st.DBError = err.Error()
		return
	}
	defer db.Close()
	st.DBLive = true
	if !tableExists(db, "aggregate_metrics") || !tableExists(db, "run_history") {
		return
	}
	var updated string
	err = db.QueryRow(`SELECT input_tokens, output_tokens, total_tokens, cache_read_tokens, seconds_running, updated_at FROM aggregate_metrics WHERE key='agent_totals'`).Scan(
		&st.AllInput, &st.AllOutput, &st.AllTotal, &st.AllCache, &st.RuntimeSec, &updated)
	if err != nil && err != sql.ErrNoRows {
		st.DBError = err.Error()
		return
	}
	rows, err := db.Query(`SELECT status, COUNT(*), COALESCE(SUM(total_tokens),0) FROM run_history GROUP BY status`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		var toks int64
		if err := rows.Scan(&status, &n, &toks); err != nil {
			continue
		}
		st.Runs += n
		switch strings.ToLower(status) {
		case "succeeded":
			st.Succeeded += n
		case "failed":
			st.Failed += n
		}
		_ = toks
	}
}

// readIssueStats aggregates run_history per issue across every known DB.
func readIssueStats(endpoints []loopEndpoint) []issueStats {
	byKey := map[string]*issueStats{}
	for _, ep := range endpoints {
		if ep.DBPath == "" {
			continue
		}
		db, err := openRO(ep.DBPath)
		if err != nil {
			continue
		}
		if !tableExists(db, "run_history") {
			db.Close()
			continue
		}
		rows, err := db.Query(`SELECT display_identifier, identifier, status, input_tokens, output_tokens, total_tokens, cache_read_tokens, completed_at FROM run_history ORDER BY id`)
		if err != nil {
			db.Close()
			continue
		}
		for rows.Next() {
			var disp sql.NullString
			var ident, status, completed string
			var in, out, tot, cache int64
			if err := rows.Scan(&disp, &ident, &status, &in, &out, &tot, &cache, &completed); err != nil {
				continue
			}
			name := ident
			if disp.Valid && disp.String != "" {
				name = disp.String
			}
			key := ep.Repo + "\x00" + ep.Loop + "\x00" + name
			s, ok := byKey[key]
			if !ok {
				s = &issueStats{Identifier: name, RepoName: ep.RepoName, Loop: ep.Loop}
				byKey[key] = s
			}
			s.Runs++
			s.Input += in
			s.Output += out
			s.Tokens += tot
			s.CacheRead += cache
			s.LastStatus = status
			s.LastRun = completed
			switch strings.ToLower(status) {
			case "succeeded":
				s.Succeeded++
			case "failed":
				s.Failed++
			}
		}
		rows.Close()
		db.Close()
	}
	out := make([]issueStats, 0, len(byKey))
	for _, s := range byKey {
		out = append(out, *s)
	}
	return out
}

func fmtTokens(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
}

func fmtDuration(sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
