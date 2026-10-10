package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"equinox/internal/card"
	"equinox/internal/store"
)

// env is what every command needs: config, vocabulary and the store.
type env struct {
	cfg   *card.Config
	vocab *card.Vocab
	st    *store.Store
}

// addConfigFlag adds --config to a command. vocab.yaml is read from the same directory.
func addConfigFlag(cmd *cobra.Command) {
	cmd.Flags().String("config", filepath.Join("config", "equinox.yaml"), "path to equinox.yaml")
}

func openEnv(cmd *cobra.Command) (*env, error) {
	path, _ := cmd.Flags().GetString("config")
	cfg, err := card.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	vocab, err := card.LoadVocab(filepath.Join(filepath.Dir(path), "vocab.yaml"))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.SQLitePath), 0o755); err != nil {
		return nil, err
	}
	st, err := store.Open(cfg.Storage.SQLitePath)
	if err != nil {
		return nil, err
	}
	return &env{cfg: cfg, vocab: vocab, st: st}, nil
}

// lockHolder names this process in the run lock, so a refusal says who holds it.
func lockHolder(command string) string {
	return fmt.Sprintf("equinox %s (pid %d)", command, os.Getpid())
}

// writing runs fn as one numbered cycle under the run lock (AGENTS.md hard rule 12).
// The cycle completes only if fn succeeds; otherwise it is marked failed and
// its rows stay invisible to readers.
func (e *env) writing(ctx context.Context, kind string, fn func(cycleID int64) error) error {
	holder := lockHolder(kind)
	if err := e.st.Lock.Acquire(ctx, holder); err != nil {
		return err
	}
	defer e.st.Lock.Release(ctx, holder)
	id, err := e.st.Cycles.Begin(ctx, kind, store.TriggerCLI)
	if err != nil {
		return err
	}
	if err := fn(id); err != nil {
		e.st.Cycles.Fail(ctx, id)
		return fmt.Errorf("cycle %d failed: %w", id, err)
	}
	return e.st.Cycles.Complete(ctx, id)
}
