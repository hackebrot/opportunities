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

// newOpportunityContactCmd is the `opps opportunity contact` parent for
// the secondary attach/detach path. Inline (during opportunity create)
// stays on `opps opportunity create --contact/--relationship`; this is
// the after-the-fact alternative.
func newOpportunityContactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contact",
		Short: "Attach or detach contacts on an opportunity",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(
		newOpportunityContactAttachCmd(),
		newOpportunityContactDetachCmd(),
	)
	return cmd
}

func newOpportunityContactAttachCmd() *cobra.Command {
	var oppFlag string
	var relationship string
	cmd := &cobra.Command{
		Use:   "attach [<contact-id>]",
		Short: "Attach a contact to an opportunity under a relationship",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			oppID, contactID, rel, err := resolveOpportunityContactArgs(
				cmd.Context(), svc, args, oppFlag, relationship, false,
			)
			if err != nil {
				return err
			}
			return svc.AttachOpportunityContact(cmd.Context(), oppID, contactID, rel)
		},
	}
	cmd.Flags().StringVar(&oppFlag, "opportunity", "", "ID of the opportunity")
	cmd.Flags().StringVar(&relationship, "as", "", "Relationship (recruiter, hiring_manager, referrer, interviewer, other)")
	return cmd
}

func newOpportunityContactDetachCmd() *cobra.Command {
	var oppFlag string
	var relationship string
	cmd := &cobra.Command{
		Use:   "detach [<contact-id>]",
		Short: "Detach a contact from an opportunity",
		Long: "Detach a contact from an opportunity. The PK is " +
			"(opportunity, contact, relationship), so --as must name the " +
			"relationship to remove — the same contact may be attached " +
			"under multiple relationships, and detach removes one row.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			oppID, contactID, rel, err := resolveOpportunityContactArgs(
				cmd.Context(), svc, args, oppFlag, relationship, true,
			)
			if err != nil {
				return err
			}
			return svc.DetachOpportunityContact(cmd.Context(), oppID, contactID, rel)
		},
	}
	cmd.Flags().StringVar(&oppFlag, "opportunity", "", "ID of the opportunity")
	cmd.Flags().StringVar(&relationship, "as", "", "Relationship to detach (required non-interactively; prompted otherwise)")
	return cmd
}

// resolveOpportunityContactArgs centralizes the picker rules shared by
// attach and detach.
//
// Interactive mode prompts for any missing piece. Non-interactive mode
// inherits the project-wide resolve convention: PickEntity auto-selects
// when exactly one row exists, otherwise fails with ErrNonInteractive —
// the caller must supply the contact arg and --opportunity. Detach
// additionally requires --as in non-interactive mode because the PK is
// the triple; the value cannot be inferred from the row count.
//
// For detach we restrict the contact picker to the contacts already
// attached and the relationship picker to the relationships actually in
// use on the picked (opportunity, contact) pair — otherwise the user
// would see options that don't correspond to any row.
func resolveOpportunityContactArgs(
	ctx context.Context,
	svc *service.Service,
	args []string,
	oppFlag, relationship string,
	detach bool,
) (oppID, contactID, rel string, err error) {
	contactID = ""
	if len(args) == 1 {
		contactID = args[0]
	}

	oppID = oppFlag
	if oppID == "" {
		opp, err := resolveOpportunity(ctx, svc, nil)
		if err != nil {
			return "", "", "", err
		}
		oppID = opp.ID
	}

	if detach {
		return resolveDetachContactAndRelationship(ctx, svc, oppID, contactID, relationship)
	}
	return resolveAttachContactAndRelationship(ctx, svc, oppID, contactID, relationship)
}

func resolveAttachContactAndRelationship(
	ctx context.Context,
	svc *service.Service,
	oppID, contactID, relationship string,
) (string, string, string, error) {
	if contactID == "" {
		contacts, err := svc.ListContacts(ctx)
		if err != nil {
			return "", "", "", err
		}
		picked, err := prompt.PickEntity(
			ctx, "Pick a contact", contacts,
			func(c model.Contact) string { return contactPickLabel(c) },
			func(c model.Contact) string { return c.ID },
		)
		if err != nil {
			return "", "", "", err
		}
		contactID = picked.ID
	}
	if relationship == "" {
		if prompt.IsNonInteractive(ctx) {
			return "", "", "", fmt.Errorf("%w: --as is required", prompt.ErrNonInteractive)
		}
		r, err := prompt.InterfaceFrom(ctx).Select("Relationship", opportunityRelationshipOptions())
		if err != nil {
			return "", "", "", err
		}
		relationship = r
	}
	return oppID, contactID, relationship, nil
}

func resolveDetachContactAndRelationship(
	ctx context.Context,
	svc *service.Service,
	oppID, contactID, relationship string,
) (string, string, string, error) {
	// detach always demands --as: the PK is the triple, and we will not
	// silently delete every row the (opp, contact) pair has just because
	// the user omitted the relationship.
	if relationship == "" && prompt.IsNonInteractive(ctx) {
		return "", "", "", fmt.Errorf("%w: --as is required", prompt.ErrNonInteractive)
	}

	attached, err := svc.ListOpportunityContacts(ctx, oppID)
	if err != nil {
		return "", "", "", err
	}
	if len(attached) == 0 {
		return "", "", "", errors.New("no contacts attached to this opportunity")
	}

	if contactID == "" {
		// Show one entry per unique contact (a contact attached as both
		// recruiter and interviewer should not appear twice in the
		// picker — the relationship choice comes after).
		uniq := uniqueAttachedContacts(attached)
		picked, err := prompt.PickEntity(
			ctx, "Pick a contact", uniq,
			func(r model.OpportunityContact) string { return r.ContactName },
			func(r model.OpportunityContact) string { return r.ContactID },
		)
		if err != nil {
			return "", "", "", err
		}
		contactID = picked.ContactID
	}

	if relationship != "" {
		return oppID, contactID, relationship, nil
	}
	// Interactive: restrict the menu to the relationships actually
	// present on (opp, contact) so the user can't pick a non-existent
	// row.
	options := relationshipsForContact(attached, contactID)
	if len(options) == 0 {
		return "", "", "", fmt.Errorf("contact %s is not attached to this opportunity", contactID)
	}
	r, err := prompt.InterfaceFrom(ctx).Select("Relationship to detach", options)
	if err != nil {
		return "", "", "", err
	}
	return oppID, contactID, r, nil
}

// uniqueAttachedContacts collapses repeated rows for the same contact
// (one per relationship) down to a single entry, preserving the
// store-supplied order so the picker matches the canonical sort.
func uniqueAttachedContacts(rows []model.OpportunityContact) []model.OpportunityContact {
	seen := make(map[string]bool, len(rows))
	out := make([]model.OpportunityContact, 0, len(rows))
	for _, r := range rows {
		if seen[r.ContactID] {
			continue
		}
		seen[r.ContactID] = true
		out = append(out, r)
	}
	return out
}

func relationshipsForContact(rows []model.OpportunityContact, contactID string) []prompt.Option {
	opts := opportunityRelationshipOptions()
	labels := make(map[string]string, len(opts))
	for _, opt := range opts {
		labels[opt.Key] = opt.Label
	}
	var out []prompt.Option
	for _, r := range rows {
		if r.ContactID != contactID {
			continue
		}
		label, ok := labels[r.Relationship]
		if !ok {
			// Forward-compat: if the schema gains a relationship value
			// the CLI's label map hasn't learned yet, show the raw key
			// rather than silently dropping the row.
			label = r.Relationship
		}
		out = append(out, prompt.Option{Key: r.Relationship, Label: label})
	}
	return out
}

// opportunityRelationshipOptions returns the full menu for attach. The
// prompt package owns the canonical interactive list, but the picker is
// only reachable from there — the CLI duplicates the keys here so it can
// build a plain Select call without exporting the prompt-package slice.
func opportunityRelationshipOptions() []prompt.Option {
	return []prompt.Option{
		{Key: "recruiter", Label: "Recruiter"},
		{Key: "hiring_manager", Label: "Hiring manager"},
		{Key: "referrer", Label: "Referrer"},
		{Key: "interviewer", Label: "Interviewer"},
		{Key: "other", Label: "Other"},
	}
}
