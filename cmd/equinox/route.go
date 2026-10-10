package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"equinox/internal/card"
	"equinox/internal/route"
	"equinox/internal/store"
)

func init() {
	cmd := &cobra.Command{
		Use:   "route",
		Short: "Route every same or opposite pair from saved prices",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sideFlag, _ := cmd.Flags().GetString("side")
			side, err := card.ParseSide(sideFlag)
			if err != nil {
				return err
			}
			// The command reads the clock for the default; the router itself never does.
			asOf := time.Now().UTC()
			if s, _ := cmd.Flags().GetString("as-of"); s != "" {
				if asOf, err = time.Parse(time.RFC3339, s); err != nil {
					return fmt.Errorf("--as-of: %w", err)
				}
			}
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			return e.writing(cmd.Context(), store.KindRoute, func(cycle int64) error {
				sum, err := route.Run(cmd.Context(), e.st, cycle, side, asOf, e.cfg)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "cycle %d: side %s as of %s: %d routed, %d no_route\n",
					cycle, side, asOf.Format(time.RFC3339), sum.Routed, sum.NoRoute)
				return nil
			})
		},
	}
	cmd.Flags().String("side", "yes", "side to buy, yes or no, stated relative to the reference venue card")
	cmd.Flags().String("as-of", "", "reference time for price age, RFC 3339 (defaults to now)")
	addConfigFlag(cmd)
	register(cmd)
}
