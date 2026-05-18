// Package cli wires cobra commands and delegates business logic to
// internal/service. Commands live in sibling files (one per aggregate).
package cli

import (
	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/prompt"
)

// nonInteractiveFlag is the persistent root flag name, referenced both
// where it's registered (NewRoot) and where it's read (WireGlobals).
const nonInteractiveFlag = "non-interactive"

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
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return WireGlobals(cmd)
		},
	}

	root.PersistentFlags().Bool(nonInteractiveFlag, false,
		"Fail instead of prompting when a required value is missing")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newDBCmd())

	return root
}

// WireGlobals reads root-level persistent flags and installs them into
// cmd's context. The root's PersistentPreRunE calls this automatically;
// any subcommand defining its own PersistentPreRun(E) MUST call this
// from there (cobra does not chain a parent's PersistentPreRun when the
// child defines its own).
func WireGlobals(cmd *cobra.Command) error {
	ni, err := cmd.Root().PersistentFlags().GetBool(nonInteractiveFlag)
	if err != nil {
		return err
	}
	cmd.SetContext(prompt.WithNonInteractive(cmd.Context(), ni))
	return nil
}
