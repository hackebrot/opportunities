package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/config"
)

func newConfigCmd() *cobra.Command {
	cfg := &cobra.Command{
		Use:   "config",
		Short: "Manage opps configuration",
		Args:  cobra.NoArgs,
	}
	cfg.AddCommand(newConfigPathCmd())
	return cfg
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the absolute path of the resolved config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	}
}
