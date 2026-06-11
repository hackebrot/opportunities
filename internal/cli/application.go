package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

// newApplicationCmd is the noun-first parent for applications.
func newApplicationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "application",
		Aliases: []string{"applications"},
		Short:   "Manage applications",
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(newApplicationFollowUpCmd())
	return cmd
}

// newApplicationFollowUpCmd implements `opps application follow-up
// [<id>] [--blocked] [--done]`. The three modes are mutually exclusive:
// no flags stamps the timestamp and emits a follow_up event; --blocked
// sets the staleness-suppression flag without an event; --done clears
// the block and stamps the timestamp.
//
// Top-level `opps follow-up` is registered as an alias in root.go so a
// single instance shares this factory.
func newApplicationFollowUpCmd() *cobra.Command {
	var blocked, done, asJSON bool
	cmd := &cobra.Command{
		Use:   "follow-up [<id>]",
		Short: "Record a follow-up, block staleness alerts, or clear a block",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if blocked && done {
				return errors.New("--blocked and --done are mutually exclusive")
			}
			mode := service.FollowUpStamp
			switch {
			case blocked:
				mode = service.FollowUpBlock
			case done:
				mode = service.FollowUpDone
			}

			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()

			app, err := resolveFollowUpApplication(cmd.Context(), svc, args, mode)
			if err != nil {
				return err
			}
			updated, err := svc.FollowUpApplication(cmd.Context(), app.ID, mode)
			if err != nil {
				return err
			}
			return printApplication(cmd.OutOrStdout(), updated, asJSON)
		},
	}
	cmd.Flags().BoolVar(&blocked, "blocked", false, "Suppress future staleness alerts for this application")
	cmd.Flags().BoolVar(&done, "done", false, "Clear a previous --blocked and stamp the follow-up timestamp")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the updated application as JSON")
	return cmd
}

// resolveFollowUpApplication returns the application identified by
// args[0] when present; otherwise picks one from the active set via
// prompt.PickEntity. The picker scope depends on mode: --done lists
// blocked apps (the user is clearing a block, so non-blocked rows are
// noise); the default and --blocked list active apps with blocked rows
// excluded. The exclusion mirrors the dashboard, which won't nag the
// user about a blocked row either.
func resolveFollowUpApplication(ctx context.Context, svc *service.Service, args []string, mode service.FollowUpMode) (model.Application, error) {
	if len(args) == 1 {
		return svc.GetApplication(ctx, args[0])
	}
	items, err := svc.ListApplications(ctx)
	if err != nil {
		return model.Application{}, err
	}
	filtered := filterFollowUpCandidates(items, mode)
	return prompt.PickEntity(
		ctx, "Pick an application", filtered,
		applicationPickLabel,
		func(a model.Application) string { return a.ID },
	)
}

// filterFollowUpCandidates trims the application list down to the rows
// the picker should offer for the given mode. The status check keeps
// terminal apps out across the board — there is nothing to follow up
// on once an app is archived.
func filterFollowUpCandidates(items []model.Application, mode service.FollowUpMode) []model.Application {
	out := make([]model.Application, 0, len(items))
	for _, a := range items {
		if !service.IsActiveAppStatus(a.Status) {
			continue
		}
		switch mode {
		case service.FollowUpDone:
			if !a.FollowUpBlocked {
				continue
			}
		default:
			if a.FollowUpBlocked {
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func applicationPickLabel(a model.Application) string {
	blocked := ""
	if a.FollowUpBlocked {
		blocked = " [blocked]"
	}
	return fmt.Sprintf("%s — %s%s", a.ID, a.Status, blocked)
}
