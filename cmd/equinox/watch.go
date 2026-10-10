package main

import (
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"equinox/internal/fetch/registry"
	"equinox/internal/watch"
)

func init() {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Run the background discovery, price and retention loops",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			if err := watch.Validate(e.cfg); err != nil {
				return err
			}
			fs, err := registry.All(e.cfg, e.vocab, &http.Client{}, time.Now)
			if err != nil {
				return err
			}
			// Display-only categories (never matched) from the same config file.
			if err := registry.LoadDisplay(e.cfgPath, fs); err != nil {
				return err
			}
			// Interrupt or SIGTERM (container stop) cancels the loop, which releases the run lock.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			w := &watch.Watcher{St: e.st, Cfg: e.cfg, Fetchers: fs, Clock: time.Now, Log: cmd.OutOrStdout(), Timeout: fetchTimeout}
			return w.Run(ctx)
		},
	}
	addConfigFlag(cmd)
	register(cmd)
}
