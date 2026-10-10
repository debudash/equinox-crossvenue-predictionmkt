// Command equinox is the CLI entrypoint. Each subcommand lives in its own file
// (fetch.go, match.go, route.go, serve.go, rebuild.go, watch.go) and registers
// itself from init() with register. Any command not yet registered gets a stub
// that exits with "not implemented", so this file never needs editing when a
// command is added.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// stubs lists every planned command and the task that owns its real implementation.
var stubs = []struct{ name, short, owner string }{
	{"fetch", "Run both fetchers and save raw responses, cards and prices", "T-20"},
	{"match", "Compare saved cards and log same, opposite or not_same", "T-12"},
	{"route", "Route every same or opposite pair from saved prices", "T-13"},
	{"serve", "Start the read-only feed", "T-21"},
	{"rebuild", "Rebuild cards from saved raw responses without calling a venue", "T-20"},
	{"watch", "Run the background discovery, price and retention loops", "T-23"},
}

var registered = map[string]*cobra.Command{}

// register adds a real subcommand. Call it from init() in the command's own file.
func register(cmd *cobra.Command) {
	registered[cmd.Name()] = cmd
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "equinox",
		Short:         "Compare Kalshi and Polymarket markets and simulate a routing choice from saved prices",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	for _, s := range stubs {
		if cmd, ok := registered[s.name]; ok {
			root.AddCommand(cmd)
			continue
		}
		root.AddCommand(stubCommand(s.name, s.short, s.owner))
	}
	return root
}

func stubCommand(name, short, owner string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   name,
		Short: short + " (not implemented)",
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("%s is not implemented yet (owned by %s)", name, owner)
		},
	}
	if name == "route" {
		cmd.Flags().String("side", "yes", "side to buy, yes or no, stated relative to the reference venue card")
		cmd.Flags().String("as-of", "", "reference time for price age, RFC 3339 (defaults to now)")
	}
	return cmd
}

func main() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "equinox:", err)
		os.Exit(1)
	}
}
