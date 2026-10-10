package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"equinox/internal/feed"
	"equinox/internal/feed/cache"
)

// serve runs the read-only feed as its own process (amendment A7). It never
// writes, so it does not take the run lock; watch can run beside it.
func init() {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the read-only feed",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			addr, _ := cmd.Flags().GetString("addr")
			every := e.cfg.Feed.CacheCheckEvery
			if every <= 0 {
				every = 5 * time.Second
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			c := cache.New(cache.FromStore(e.st), e.cfg.Feed.KeepSnapshots)
			if _, err := c.Refresh(ctx); err != nil {
				return err
			}
			out := cmd.ErrOrStderr()
			go c.Run(ctx, every, func(err error) { fmt.Fprintln(out, "feed cache refresh failed, keeping last snapshot:", err) })

			srv := &http.Server{Addr: addr, Handler: feed.New(c, e.cfg.Feed.PageSize).Handler(), ReadHeaderTimeout: 10 * time.Second}
			go func() {
				<-ctx.Done()
				shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				srv.Shutdown(shutdown)
			}()
			fmt.Fprintf(cmd.OutOrStdout(), "feed on %s, checking for new cycles every %s\n", addr, every)
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		},
	}
	cmd.Flags().String("addr", ":8080", "address to listen on")
	addConfigFlag(cmd)
	register(cmd)
}
