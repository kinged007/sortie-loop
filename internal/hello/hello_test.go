package hello

import "testing"

func TestWorld(t *testing.T) {
	if got := World(); got != "hello world" {
		t.Fatalf("got %q", got)
	}
}
