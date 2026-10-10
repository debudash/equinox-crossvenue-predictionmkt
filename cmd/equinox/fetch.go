package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"equinox/internal/fetch/registry"
	"equinox/internal/store"
)

// fetchTimeout bounds each venue's whole run; each fetcher also times out every request.
const fetchTimeout = 3 * time.Minute

func init() {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Run both fetchers and save raw responses, cards and prices",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			ctx := cmd.Context()
			fs, err := registry.All(e.cfg, e.vocab, &http.Client{}, time.Now)
			if err != nil {
				return err
			}
			// Display-only categories (never matched) from the same config file.
			if err := registry.LoadDisplay(e.cfgPath, fs); err != nil {
				return err
			}
			return e.writing(ctx, store.KindFetch, func(cycle int64) error {
				out := cmd.OutOrStdout()
				for _, r := range registry.FetchAll(ctx, e.st, cycle, fs, fetchTimeout) {
					if r.Err != nil {
						return fmt.Errorf("%s: %w", r.Venue, r.Err)
					}
					fmt.Fprintf(out, "%-12s %-7s saved %d, skipped %d", r.Venue, r.Run.Status, r.Run.Saved, r.Run.Skipped)
					if r.Run.Error != "" {
						fmt.Fprintf(out, ", error: %s", r.Run.Error)
					}
					fmt.Fprintln(out)
				}
				skips, err := e.st.Skips.ForCycle(ctx, cycle)
				if err != nil {
					return err
				}
				reasons := map[string]int{}
				for _, k := range skips {
					reasons[k.Venue+": "+k.Reason]++
				}
				for reason, n := range reasons {
					fmt.Fprintf(out, "  skipped %d x %s\n", n, reason)
				}
				fmt.Fprintf(out, "cycle %d complete\n", cycle)
				return nil
			})
		},
	}
	addConfigFlag(cmd)
	register(cmd)
}
