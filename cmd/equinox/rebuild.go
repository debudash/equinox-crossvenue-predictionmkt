package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"equinox/internal/fetch/registry"
	"equinox/internal/store"
)

func init() {
	cmd := &cobra.Command{
		Use:   "rebuild",
		Short: "Rebuild cards from saved raw responses without calling a venue",
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := openEnv(cmd)
			if err != nil {
				return err
			}
			defer e.st.Close()
			return e.writing(cmd.Context(), store.KindRebuild, func(cycle int64) error {
				sum, err := registry.Rebuild(cmd.Context(), e.st, cycle, e.vocab)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "cycle %d: rebuilt %d card versions from %d raw responses; %d records skipped; no venue called\n",
					cycle, sum.Versions, sum.Raws, sum.Skipped)
				return nil
			})
		},
	}
	addConfigFlag(cmd)
	register(cmd)
}
