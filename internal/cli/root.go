// Package cli wires cobra commands and delegates business logic to
// internal/service. Commands live in sibling files (one per aggregate).
//
// CLI grammar is noun-first: top-level commands are entities
// (`opps company`, `opps contact`, …) with CRUD verbs as subcommands
// (`create`, `list`, `show`, `update`, `rm`). Top-level verbs are
// reserved for app-level operations (`version`, `db`, `config`, …) and
// a small fixed set of user-story-justified shortcut aliases.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/prompt"
)

// Run every PersistentPreRunE in the parent chain, not just the most
// specific one. Without this, a subcommand defining its own
// PersistentPreRunE would silently shadow the root's flag-wiring hook.
// Set in init() because it's a package-level global in cobra; assigning
// it inside NewRoot races under -race when tests build multiple roots.
func init() {
	cobra.EnableTraverseRunHooks = true
}

// NewRoot builds the root `opps` command tree. version is the binary
// version string (set via -ldflags "-X main.version=..." in cmd/opps).
func NewRoot(version string) *cobra.Command {
	var nonInteractive bool

	root := &cobra.Command{
		Use:   "opps",
		Short: "Local-first CLI for tracking job opportunities and applications",
		// main owns error printing (one "opps: <err>" line) and runtime
		// failures don't warrant dumping the usage screen. Cobra still
		// prints usage on flag-parse errors regardless of SilenceUsage.
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SetContext(prompt.WithNonInteractive(cmd.Context(), nonInteractive))
			return nil
		},
	}

	root.PersistentFlags().BoolVar(&nonInteractive, "non-interactive", false,
		"Fail instead of prompting when a required value is missing")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newDBCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newCompanyCmd())
	root.AddCommand(newContactCmd())
	root.AddCommand(newOpportunityCmd())

	return root
}
