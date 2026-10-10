package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"equinox/internal/match"
	"equinox/internal/store"
)

func init() {
	cmd := &cobra.Command{
		Use:   "match",
		Short: "Compare saved cards and log same, opposite or not_same",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			return e.writing(cmd.Context(), store.KindMatch, func(cycle int64) error {
				sum, err := match.Run(cmd.Context(), e.st, cycle, e.cfg)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				for _, c := range sum.Counts {
					subject := c.Subject
					if subject == "" {
						subject = "(empty subject)"
					}
					fmt.Fprintf(out, "cards: %-24s %-12s %d\n", subject, c.Venue, c.Count)
				}
				fmt.Fprintf(out, "cycle %d: compared %d pairs: %d same, %d opposite, %d not_same; %d cards skipped\n",
					cycle, sum.Compared, sum.Same, sum.Opposite, sum.NotSame, sum.Skipped)
				return nil
			})
		},
	}
	addConfigFlag(cmd)
	register(cmd)
}
