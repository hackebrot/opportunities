package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

// applicationFlags mirrors the editable subset of service.ApplicationInput.
// On update, cmd.Flags().Changed is the source of truth for "user
// supplied this flag" — both omitted flags and explicit `--foo=""`
// deserialize to the empty string here.
//
// opportunity is intentionally absent from this struct: applications
// cannot be re-parented to a different opportunity (events.opportunity_id
// would be orphaned), so --opportunity is bound on create only.
type applicationFlags struct {
	appliedAt        string
	appliedWithEmail string
	notes            string
}

func bindApplicationFlags(cmd *cobra.Command, f *applicationFlags) {
	cmd.Flags().StringVar(&f.appliedAt, "applied-at", "", "Submission time as RFC3339 (empty clears on update)")
	cmd.Flags().StringVar(&f.appliedWithEmail, "applied-with-email", "", "Email used on the application (optional override)")
	cmd.Flags().StringVar(&f.notes, "notes", "", "Free-form notes")
}

// newApplicationCmd is the noun-first parent for applications.
func newApplicationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "application",
		Aliases: []string{"applications"},
		Short:   "Manage applications",
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newApplicationCreateCmd(),
		newApplicationListCmd(),
		newApplicationShowCmd(),
		newApplicationUpdateCmd(),
		newApplicationRmCmd(),
		newApplicationFollowUpCmd(),
	)
	return cmd
}

func newApplicationCreateCmd() *cobra.Command {
	var f applicationFlags
	var opportunity string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an application (picks or creates an opportunity inline)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			in := service.ApplicationInput{
				OpportunityID:    opportunity,
				AppliedWithEmail: f.appliedWithEmail,
				Notes:            f.notes,
			}
			if f.appliedAt != "" {
				t, err := time.Parse(time.RFC3339, f.appliedAt)
				if err != nil {
					return fmt.Errorf("--applied-at: %w", err)
				}
				in.AppliedAt = &t
			}
			app, err := prompt.AddApplication(cmd.Context(), svc, prompt.ApplicationCreationInput{
				Application: in,
			})
			if err != nil {
				return err
			}
			return printApplication(cmd.OutOrStdout(), app, asJSON)
		},
	}
	cmd.Flags().StringVar(&opportunity, "opportunity", "", "ID of the opportunity (required when creating non-interactively)")
	bindApplicationFlags(cmd, &f)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the created application as JSON")
	return cmd
}

func newApplicationListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List applications",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			items, err := svc.ListApplications(cmd.Context())
			if err != nil {
				return err
			}
			return printApplications(cmd.OutOrStdout(), items, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newApplicationShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show [<id>]",
		Short: "Show an application",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			app, err := resolveApplication(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			return printApplication(cmd.OutOrStdout(), app, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newApplicationUpdateCmd() *cobra.Command {
	var f applicationFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update [<id>]",
		Short: "Update an application",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			current, err := resolveApplication(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			in := applicationInputFromModel(current)
			if cmd.Flags().Changed("applied-at") {
				if f.appliedAt == "" {
					in.AppliedAt = nil
				} else {
					t, err := time.Parse(time.RFC3339, f.appliedAt)
					if err != nil {
						return fmt.Errorf("--applied-at: %w", err)
					}
					in.AppliedAt = &t
				}
			}
			if cmd.Flags().Changed("applied-with-email") {
				in.AppliedWithEmail = f.appliedWithEmail
			}
			if cmd.Flags().Changed("notes") {
				in.Notes = f.notes
			}
			updated, err := svc.UpdateApplication(cmd.Context(), current.ID, in)
			if err != nil {
				return err
			}
			return printApplication(cmd.OutOrStdout(), updated, asJSON)
		},
	}
	bindApplicationFlags(cmd, &f)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newApplicationRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm [<id>]",
		Short: "Delete an application",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			app, err := resolveApplication(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := prompt.Confirm(cmd.Context(),
					fmt.Sprintf("Delete application %s (%s)?", app.ID, app.Status))
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("delete aborted")
				}
			}
			return svc.DeleteApplication(cmd.Context(), app.ID)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

// resolveApplication returns the application identified by args[0] when
// present; otherwise it picks one via prompt.PickEntity, which
// auto-selects when exactly one application exists.
func resolveApplication(ctx context.Context, svc *service.Service, args []string) (model.Application, error) {
	if len(args) == 1 {
		return svc.GetApplication(ctx, args[0])
	}
	items, err := svc.ListApplications(ctx)
	if err != nil {
		return model.Application{}, err
	}
	return prompt.PickEntity(
		ctx, "Pick an application", items,
		applicationPickLabel,
		func(a model.Application) string { return a.ID },
	)
}

// applicationInputFromModel seeds an update with the current values so
// unchanged flags round-trip. Status-machine columns (status, archived_at,
// archive_reason*) are deliberately excluded — those belong to the events
// engine.
func applicationInputFromModel(a model.Application) service.ApplicationInput {
	return service.ApplicationInput{
		OpportunityID:    a.OpportunityID,
		AppliedAt:        a.AppliedAt,
		AppliedWithEmail: a.AppliedWithEmail,
		Notes:            a.Notes,
	}
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

func printApplications(w io.Writer, items []model.Application, asJSON bool) error {
	if asJSON {
		out := make([]applicationJSON, len(items))
		for i, a := range items {
			out[i] = toApplicationJSON(a)
		}
		return writeJSON(w, out)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tOPPORTUNITY\tSTATUS\tAPPLIED AT\tEMAIL"); err != nil {
		return err
	}
	for _, a := range items {
		appliedAt := ""
		if a.AppliedAt != nil {
			appliedAt = a.AppliedAt.UTC().Format(time.RFC3339)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			a.ID, a.OpportunityID, a.Status, appliedAt, oneline(a.AppliedWithEmail)); err != nil {
			return err
		}
	}
	return tw.Flush()
}
