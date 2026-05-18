// Package cli wires cobra commands and delegates business logic to
// internal/service. Commands live in sibling files (one per aggregate).
package cli

import "github.com/spf13/cobra"

// NewRoot builds the root `opps` command tree. version is the binary
// version string (set via -ldflags "-X main.version=..." in cmd/opps).
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "opps",
		Short: "Local-first CLI for tracking job opportunities and applications",
		// main owns error printing (one "opps: <err>" line) and runtime
		// failures don't warrant dumping the usage screen. Cobra still
		// prints usage on flag-parse errors regardless of SilenceUsage.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newDBCmd())

	return root
}
