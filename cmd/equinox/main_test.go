package main

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRootHasEveryCommand(t *testing.T) {
	root := newRoot()
	for _, s := range stubs {
		cmd, _, err := root.Find([]string{s.name})
		if err != nil || cmd.Name() != s.name {
			t.Fatalf("command %q not found: %v", s.name, err)
		}
	}
	route, _, _ := root.Find([]string{"route"})
	for _, f := range []string{"side", "as-of"} {
		if route.Flags().Lookup(f) == nil {
			t.Errorf("route stub missing --%s", f)
		}
	}
}

func TestStubReportsOwner(t *testing.T) {
	err := stubCommand("later", "a planned command", "T-99").RunE(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not implemented") || !strings.Contains(err.Error(), "T-99") {
		t.Fatalf("want not-implemented error naming T-99, got %v", err)
	}
}

func TestRegisteredCommandReplacesStub(t *testing.T) {
	ran := false
	real := registered["serve"]
	register(&cobra.Command{Use: "serve", RunE: func(*cobra.Command, []string) error { ran = true; return nil }})
	t.Cleanup(func() { registered["serve"] = real })
	root := newRoot()
	root.SetArgs([]string{"serve"})
	if err := root.Execute(); err != nil || !ran {
		t.Fatalf("registered command did not run: %v", err)
	}
}
