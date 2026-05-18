package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/config"
	"github.com/hackebrot/opportunities/internal/store"
)

func newDBCmd() *cobra.Command {
	db := &cobra.Command{
		Use:   "db",
		Short: "Database administration (migrations, reset)",
		Args:  cobra.NoArgs,
	}

	migrate := &cobra.Command{
		Use:   "migrate",
		Short: "Schema migration operations",
		Args:  cobra.NoArgs,
	}
	migrate.AddCommand(
		newDBMigrateSubcmd("up", "Apply every pending migration", (*store.Store).MigrateUp),
		newDBMigrateSubcmd("down", "Roll back the most recently applied migration", (*store.Store).MigrateDown),
		newDBMigrateSubcmd("status", "Print applied/pending migrations", (*store.Store).MigrateStatus),
		newDBMigrateSubcmd("redo", "Roll back and re-apply the most recent migration", (*store.Store).MigrateRedo),
	)

	db.AddCommand(migrate, newDBResetCmd())
	return db
}

func newDBMigrateSubcmd(use, short string, op func(*store.Store, context.Context) error) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openStoreFromConfig(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			return op(s, cmd.Context())
		},
	}
}

func newDBResetCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Drop the schema and re-apply every migration (destructive)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !yes {
				return errors.New("refusing to reset without --yes (this destroys all data)")
			}
			s, err := openStoreFromConfig(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			return s.Reset(cmd.Context())
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm the destructive reset")
	return cmd
}

func openStoreFromConfig(cmd *cobra.Command) (*store.Store, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("db: load config: %w", err)
	}
	s, err := store.Open(cmd.Context(), cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("db: open store: %w", err)
	}
	return s, nil
}
