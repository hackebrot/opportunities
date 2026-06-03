package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

// opportunityFlags mirrors service.OpportunityInput. On update,
// cmd.Flags().Changed is the source of truth for "user supplied this
// flag" — both omitted flags and explicit `--foo=""` deserialize to the
// empty string / 0 here.
type opportunityFlags struct {
	company           string
	roleTitle         string
	location          string
	officeDaysPerWeek int
	source            string
	sourceDetail      string
	priority          string
	notes             string
}

func bindOpportunityFlags(cmd *cobra.Command, f *opportunityFlags) {
	cmd.Flags().StringVar(&f.company, "company", "", "ID of the company (required when creating non-interactively)")
	cmd.Flags().StringVar(&f.roleTitle, "role-title", "", "Role title (optional; can be filled in later)")
	cmd.Flags().StringVar(&f.location, "location", "", "Location")
	cmd.Flags().IntVar(&f.officeDaysPerWeek, "office-days-per-week", prompt.OfficeDaysUnset, "Office days per week, 0=remote, 5=onsite")
	cmd.Flags().StringVar(&f.source, "source", "", "How the opportunity was sourced (outbound, inbound_inhouse_recruiter, inbound_external_recruiter, inbound_founder, inbound_employee, referral, network, other)")
	cmd.Flags().StringVar(&f.sourceDetail, "source-detail", "", "Free-form source detail")
	cmd.Flags().StringVar(&f.priority, "priority", "", "Priority (low, normal, high)")
	cmd.Flags().StringVar(&f.notes, "notes", "", "Free-form notes")
}

// newOpportunityCmd is the noun-first parent for opportunities.
func newOpportunityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "opportunity",
		Aliases: []string{"opportunities"},
		Short:   "Manage opportunities",
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newOpportunityCreateCmd(),
		newOpportunityListCmd(),
		newOpportunityShowCmd(),
		newOpportunityUpdateCmd(),
		newOpportunityRmCmd(),
		newOpportunityArchiveCmd(),
		newOpportunityNoteCmd(),
		newOpportunityEventCmd(),
		newOpportunityContactCmd(),
	)
	return cmd
}

func newOpportunityCreateCmd() *cobra.Command {
	var f opportunityFlags
	var contact string
	var relationship string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an opportunity (picks or creates company and contact inline)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()

			in := service.OpportunityCreationInput{
				Company: service.OpportunityCompanyChoice{ID: f.company},
				Opportunity: service.OpportunityInput{
					RoleTitle:         f.roleTitle,
					Location:          f.location,
					OfficeDaysPerWeek: f.officeDaysPerWeek,
					Source:            f.source,
					SourceDetail:      f.sourceDetail,
					Priority:          f.priority,
					Notes:             f.notes,
				},
			}
			if cmd.Flags().Changed("contact") || cmd.Flags().Changed("relationship") {
				if contact == "" || relationship == "" {
					return errors.New("--contact and --relationship must be set together")
				}
				in.Contact = &service.OpportunityContactChoice{
					ID:           contact,
					Relationship: relationship,
				}
			}
			opp, err := prompt.AddOpportunity(cmd.Context(), svc, in)
			if err != nil {
				return err
			}
			return printOpportunity(cmd.OutOrStdout(), opp, asJSON)
		},
	}
	bindOpportunityFlags(cmd, &f)
	cmd.Flags().StringVar(&contact, "contact", "", "ID of an existing contact to attach (requires --relationship)")
	cmd.Flags().StringVar(&relationship, "relationship", "", "Relationship for the attached contact (recruiter, hiring_manager, referrer, interviewer, other)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the created opportunity as JSON")
	return cmd
}

func newOpportunityListCmd() *cobra.Command {
	var asJSON bool
	var sortBy string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List opportunities",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			items, err := svc.ListOpportunities(cmd.Context())
			if err != nil {
				return err
			}
			if err := sortOpportunities(items, sortBy); err != nil {
				return err
			}
			return printOpportunities(cmd.OutOrStdout(), items, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	cmd.Flags().StringVar(&sortBy, "sort", "created", "Sort order: created (most recent first) or status (latest_status, then created)")
	return cmd
}

func newOpportunityShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show [<id>]",
		Short: "Show an opportunity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			opp, err := resolveOpportunity(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			return printOpportunity(cmd.OutOrStdout(), opp, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newOpportunityUpdateCmd() *cobra.Command {
	var f opportunityFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update [<id>]",
		Short: "Update an opportunity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			current, err := resolveOpportunity(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			in := opportunityInputFromModel(current)
			if cmd.Flags().Changed("company") {
				in.CompanyID = f.company
			}
			if cmd.Flags().Changed("role-title") {
				in.RoleTitle = f.roleTitle
			}
			if cmd.Flags().Changed("location") {
				in.Location = f.location
			}
			if cmd.Flags().Changed("office-days-per-week") {
				in.OfficeDaysPerWeek = f.officeDaysPerWeek
			}
			if cmd.Flags().Changed("source") {
				in.Source = f.source
			}
			if cmd.Flags().Changed("source-detail") {
				in.SourceDetail = f.sourceDetail
			}
			if cmd.Flags().Changed("priority") {
				in.Priority = f.priority
			}
			if cmd.Flags().Changed("notes") {
				in.Notes = f.notes
			}
			updated, err := svc.UpdateOpportunity(cmd.Context(), current.ID, in)
			if err != nil {
				return err
			}
			return printOpportunity(cmd.OutOrStdout(), updated, asJSON)
		},
	}
	bindOpportunityFlags(cmd, &f)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newOpportunityRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm [<id>]",
		Short: "Delete an opportunity",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			opp, err := resolveOpportunity(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := prompt.Confirm(cmd.Context(),
					fmt.Sprintf("Delete opportunity %q (%s)?", opportunityDisplayName(opp), opp.ID))
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("delete aborted")
				}
			}
			return svc.DeleteOpportunity(cmd.Context(), opp.ID)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

func newOpportunityArchiveCmd() *cobra.Command {
	var reason string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "archive [<id>]",
		Short: "Archive an opportunity (terminal state)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			opp, err := resolveOpportunity(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if _, err := svc.ArchiveOpportunity(cmd.Context(), opp.ID, reason); err != nil {
				return err
			}
			refreshed, err := svc.GetOpportunity(cmd.Context(), opp.ID)
			if err != nil {
				return err
			}
			return printOpportunity(cmd.OutOrStdout(), refreshed, asJSON)
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Free-form archive reason")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newOpportunityNoteCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "note [<id>] [<text>]",
		Short: "Append a free-form note event to an opportunity",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			oppArgs, noteText := splitNoteArgs(args)
			opp, err := resolveOpportunity(cmd.Context(), svc, oppArgs)
			if err != nil {
				return err
			}
			if noteText == "" {
				if err := prompt.Text(cmd.Context(), "Note", &noteText); err != nil {
					return err
				}
			}
			ev, err := svc.AppendEvent(cmd.Context(), service.EventInput{
				OpportunityID: opp.ID,
				Kind:          "note",
				Notes:         noteText,
			})
			if err != nil {
				return err
			}
			return printEvent(cmd.OutOrStdout(), ev, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newOpportunityEventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Append a timeline event to an opportunity",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newOpportunityEventCreateCmd())
	return cmd
}

func newOpportunityEventCreateCmd() *cobra.Command {
	var kind string
	var label string
	var notes string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create [<id>]",
		Short: "Append a timeline event (kinds: exploring, archived, note, follow_up, custom, declined)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			opp, err := resolveOpportunity(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if kind == "" {
				return errors.New("--kind is required")
			}
			ev, err := svc.AppendEvent(cmd.Context(), service.EventInput{
				OpportunityID: opp.ID,
				Kind:          kind,
				Label:         label,
				Notes:         notes,
			})
			if err != nil {
				return err
			}
			return printEvent(cmd.OutOrStdout(), ev, asJSON)
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Event kind (exploring, archived, note, follow_up, custom, declined)")
	cmd.Flags().StringVar(&label, "label", "", "Required free-form label when --kind=custom")
	cmd.Flags().StringVar(&notes, "notes", "", "Free-form notes")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

// splitNoteArgs disambiguates `opps opportunity note <id> <text>` from
// `opps opportunity note <text>`. The id is opaque to the CLI here
// (resolveOpportunity validates via svc.GetOpportunity), so we cannot
// safely distinguish a UUID from a single-word note. Convention: pass
// the id when supplying both; with a single positional, treat it as the
// note text and let resolveOpportunity prompt or auto-select.
// Multi-word note text must be quoted — cobra's MaximumNArgs(2) rejects
// more than two positionals.
func splitNoteArgs(args []string) ([]string, string) {
	switch len(args) {
	case 0:
		return nil, ""
	case 1:
		return nil, args[0]
	default:
		return []string{args[0]}, args[1]
	}
}

// sortOpportunities orders by the requested key. The store already
// returns rows by created_at DESC, so "created" is a no-op; "status"
// groups by latest_status (alphabetical) and breaks ties by created_at
// DESC.
func sortOpportunities(items []model.Opportunity, sortBy string) error {
	switch sortBy {
	case "", "created":
		return nil
	case "status":
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].LatestStatus != items[j].LatestStatus {
				return items[i].LatestStatus < items[j].LatestStatus
			}
			return items[i].CreatedAt.After(items[j].CreatedAt)
		})
		return nil
	default:
		return fmt.Errorf("--sort: unknown key %q (want created|status)", sortBy)
	}
}

// resolveOpportunity returns the opportunity identified by args[0] when
// present; otherwise picks one via prompt.PickEntity (auto-selects on a
// single row, errors non-interactively on more than one).
func resolveOpportunity(ctx context.Context, svc *service.Service, args []string) (model.Opportunity, error) {
	if len(args) == 1 {
		return svc.GetOpportunity(ctx, args[0])
	}
	items, err := svc.ListOpportunities(ctx)
	if err != nil {
		return model.Opportunity{}, err
	}
	return prompt.PickEntity(
		ctx, "Pick an opportunity", items,
		func(o model.Opportunity) string { return opportunityPickLabel(o) },
		func(o model.Opportunity) string { return o.ID },
	)
}

func opportunityPickLabel(o model.Opportunity) string {
	role := "(no role)"
	if o.RoleTitle != nil && *o.RoleTitle != "" {
		role = *o.RoleTitle
	}
	return fmt.Sprintf("%s — %s [%s]", o.CompanyName, role, o.LatestStatus)
}

func opportunityDisplayName(o model.Opportunity) string {
	if o.RoleTitle != nil && *o.RoleTitle != "" {
		return fmt.Sprintf("%s @ %s", *o.RoleTitle, o.CompanyName)
	}
	return o.CompanyName
}

func opportunityInputFromModel(o model.Opportunity) service.OpportunityInput {
	in := service.OpportunityInput{
		CompanyID:         o.CompanyID,
		Location:          o.Location,
		OfficeDaysPerWeek: o.OfficeDaysPerWeek,
		Source:            o.Source,
		SourceDetail:      o.SourceDetail,
		Priority:          o.Priority,
		Notes:             o.Notes,
	}
	if o.RoleTitle != nil {
		in.RoleTitle = *o.RoleTitle
	}
	return in
}

// opportunityJSON shapes the on-the-wire representation.
type opportunityJSON struct {
	ID                string  `json:"id"`
	CompanyID         string  `json:"company_id"`
	CompanyName       string  `json:"company_name"`
	RoleTitle         *string `json:"role_title,omitempty"`
	Location          string  `json:"location,omitempty"`
	OfficeDaysPerWeek int     `json:"office_days_per_week"`
	Source            string  `json:"source"`
	SourceDetail      string  `json:"source_detail,omitempty"`
	Priority          string  `json:"priority"`
	LatestStatus      string  `json:"latest_status"`
	ArchivedAt        *string `json:"archived_at,omitempty"`
	ArchiveReason     *string `json:"archive_reason,omitempty"`
	Notes             string  `json:"notes,omitempty"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

func toOpportunityJSON(o model.Opportunity) opportunityJSON {
	out := opportunityJSON{
		ID:                o.ID,
		CompanyID:         o.CompanyID,
		CompanyName:       o.CompanyName,
		RoleTitle:         o.RoleTitle,
		Location:          o.Location,
		OfficeDaysPerWeek: o.OfficeDaysPerWeek,
		Source:            o.Source,
		SourceDetail:      o.SourceDetail,
		Priority:          o.Priority,
		LatestStatus:      o.LatestStatus,
		ArchiveReason:     o.ArchiveReason,
		Notes:             o.Notes,
		CreatedAt:         o.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         o.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if o.ArchivedAt != nil {
		s := o.ArchivedAt.UTC().Format(time.RFC3339)
		out.ArchivedAt = &s
	}
	return out
}

func printOpportunity(w io.Writer, o model.Opportunity, asJSON bool) error {
	if asJSON {
		return writeJSON(w, toOpportunityJSON(o))
	}
	role := ""
	if o.RoleTitle != nil {
		role = *o.RoleTitle
	}
	archivedAt := ""
	if o.ArchivedAt != nil {
		archivedAt = o.ArchivedAt.UTC().Format(time.RFC3339)
	}
	archiveReason := ""
	if o.ArchiveReason != nil {
		archiveReason = *o.ArchiveReason
	}
	rows := [][2]string{
		{"ID", o.ID},
		{"Company", oneline(o.CompanyName)},
		{"Role", oneline(role)},
		{"Location", oneline(o.Location)},
		{"Office days/week", fmt.Sprintf("%d", o.OfficeDaysPerWeek)},
		{"Source", o.Source},
		{"Source detail", oneline(o.SourceDetail)},
		{"Priority", o.Priority},
		{"Latest status", o.LatestStatus},
		{"Archived at", archivedAt},
		{"Archive reason", oneline(archiveReason)},
		{"Notes", oneline(o.Notes)},
		{"Created", o.CreatedAt.UTC().Format(time.RFC3339)},
		{"Updated", o.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", r[0], r[1]); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printOpportunities(w io.Writer, items []model.Opportunity, asJSON bool) error {
	if asJSON {
		out := make([]opportunityJSON, len(items))
		for i, o := range items {
			out[i] = toOpportunityJSON(o)
		}
		return writeJSON(w, out)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tCOMPANY\tROLE\tSTATUS\tSOURCE\tPRIORITY"); err != nil {
		return err
	}
	for _, o := range items {
		role := ""
		if o.RoleTitle != nil {
			role = *o.RoleTitle
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			o.ID, oneline(o.CompanyName), oneline(role), o.LatestStatus, o.Source, o.Priority); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// eventJSON shapes the on-the-wire event payload used by `note` and
// `event create` output.
type eventJSON struct {
	ID            string  `json:"id"`
	OpportunityID string  `json:"opportunity_id"`
	ApplicationID *string `json:"application_id,omitempty"`
	Kind          string  `json:"kind"`
	OccurredAt    string  `json:"occurred_at"`
	Label         *string `json:"label,omitempty"`
	Notes         string  `json:"notes,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

func toEventJSON(e model.Event) eventJSON {
	return eventJSON{
		ID:            e.ID,
		OpportunityID: e.OpportunityID,
		ApplicationID: e.ApplicationID,
		Kind:          e.Kind,
		OccurredAt:    e.OccurredAt.UTC().Format(time.RFC3339),
		Label:         e.Label,
		Notes:         e.Notes,
		CreatedAt:     e.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func printEvent(w io.Writer, e model.Event, asJSON bool) error {
	if asJSON {
		return writeJSON(w, toEventJSON(e))
	}
	label := ""
	if e.Label != nil {
		label = *e.Label
	}
	rows := [][2]string{
		{"ID", e.ID},
		{"Opportunity", e.OpportunityID},
		{"Kind", e.Kind},
		{"Label", oneline(label)},
		{"Notes", oneline(e.Notes)},
		{"Occurred", e.OccurredAt.UTC().Format(time.RFC3339)},
		{"Created", e.CreatedAt.UTC().Format(time.RFC3339)},
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", r[0], r[1]); err != nil {
			return err
		}
	}
	return tw.Flush()
}
