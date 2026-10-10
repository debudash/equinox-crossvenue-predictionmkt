package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// unlock clears a run lock left behind by a crashed writer (decision Q-2).
// It is deliberately explicit: the user names the holder shown in the refusal,
// so a live writer's lock is never cleared by accident.
func init() {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "Clear a run lock left by a crashed writer",
		Long: "Shows who holds the run lock. With --holder set to exactly that name, clears it.\n" +
			"Use only when that process is no longer running.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			ctx := cmd.Context()
			holder, since, held, err := e.st.Lock.Holder(ctx)
			if err != nil {
				return err
			}
			if !held {
				fmt.Fprintln(cmd.OutOrStdout(), "run lock is free")
				return nil
			}
			want, _ := cmd.Flags().GetString("holder")
			if want != holder {
				return fmt.Errorf("run lock held by %q since %s; rerun with --holder %q to clear it",
					holder, since.UTC().Format(time.RFC3339), holder)
			}
			if err := e.st.Lock.Release(ctx, holder); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cleared run lock held by %q\n", holder)
			return nil
		},
	}
	addConfigFlag(cmd)
	cmd.Flags().String("holder", "", "exact holder name to clear, as shown by unlock without flags")
	register(cmd)
}
