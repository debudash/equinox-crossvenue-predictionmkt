package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"equinox/internal/store"
	"equinox/internal/testkit"
)

// writeConfig makes a config whose database lives in a temp directory.
func writeConfig(t *testing.T) (cfgPath, dbPath string) {
	t.Helper()
	dir := t.TempDir()
	root := testkit.RepoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "config", "equinox.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dbPath = filepath.Join(dir, "equinox.db")
	cfg := strings.Replace(string(src), "sqlite_path: data/equinox.db", "sqlite_path: "+filepath.ToSlash(dbPath), 1)
	vocab, _ := os.ReadFile(filepath.Join(root, "config", "vocab.yaml"))
	cfgPath = filepath.Join(dir, "equinox.yaml")
	os.WriteFile(cfgPath, []byte(cfg), 0o644)
	os.WriteFile(filepath.Join(dir, "vocab.yaml"), vocab, 0o644)
	return cfgPath, dbPath
}

func TestAmd_W7_CliRefusedWhileWatchHoldsLock(t *testing.T) {
	cfgPath, dbPath := writeConfig(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := st.Lock.Acquire(ctx, "equinox watch (pid 4242)"); err != nil {
		t.Fatal(err)
	}
	st.Close()

	for _, args := range [][]string{{"match"}, {"route", "--side", "yes"}, {"rebuild"}, {"fetch"}} {
		root := newRoot()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(append(args, "--config", cfgPath))
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "refused") || !strings.Contains(err.Error(), "equinox watch (pid 4242)") {
			t.Errorf("%v: want refusal naming the holder, got %v", args, err)
		}
	}

	st, _ = store.Open(dbPath)
	defer st.Close()
	if _, ok, _ := st.Cycles.LatestCompleted(ctx); ok {
		t.Error("a refused command must write nothing")
	}
	if c, err := st.Cycles.Get(ctx, 1); err == nil {
		t.Errorf("a refused command must not open a cycle: %+v", c)
	}
}

func TestUnlockClearsOnlyTheNamedHolder(t *testing.T) {
	cfgPath, dbPath := writeConfig(t)
	st, _ := store.Open(dbPath)
	st.Lock.Acquire(context.Background(), "equinox watch (pid 7)")
	st.Close()

	run := func(args ...string) (string, error) {
		root := newRoot()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(append([]string{"unlock", "--config", cfgPath}, args...))
		err := root.Execute()
		return out.String(), err
	}
	if _, err := run(); err == nil || !strings.Contains(err.Error(), "equinox watch (pid 7)") {
		t.Fatalf("unlock without --holder must name the holder and refuse, got %v", err)
	}
	if _, err := run("--holder", "someone else"); err == nil {
		t.Fatal("wrong holder must not clear the lock")
	}
	if out, err := run("--holder", "equinox watch (pid 7)"); err != nil || !strings.Contains(out, "cleared") {
		t.Fatalf("named holder must clear: %q %v", out, err)
	}
	if out, _ := run(); !strings.Contains(out, "free") {
		t.Errorf("lock must be free: %q", out)
	}
}
