package main

import (
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestFindUniteDashboard(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<title>Sortie Loop \u2014 Unite</title>")
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	defer srv.Close()

	// Temporarily narrow the scan range to our ephemeral port by
	// checking the marker logic directly: a unite page is found,
	// a loop page is not.
	if got := findUniteDashboardOn(port); got != port {
		t.Errorf("unite page: got %d, want %d", got, port)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port2 := ln2.Addr().(*net.TCPAddr).Port
	mux2 := http.NewServeMux()
	mux2.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<title>Sortie Dashboard</title>")
	})
	srv2 := &http.Server{Handler: mux2}
	go srv2.Serve(ln2)
	defer srv2.Close()
	if got := findUniteDashboardOn(port2); got != 0 {
		t.Errorf("non-unite page: got %d, want 0", got)
	}
}
