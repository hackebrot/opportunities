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

// contactFlags mirrors service.ContactInput. On update, cmd.Flags().Changed
// is the source of truth for "user supplied this flag" — both omitted
// flags and explicit `--foo=""` deserialize to the empty string here.
type contactFlags struct {
	name     string
	email    string
	linkedin string
	role     string
	company  string
	notes    string
}

func bindContactFlags(cmd *cobra.Command, f *contactFlags, includeName bool) {
	if includeName {
		cmd.Flags().StringVar(&f.name, "name", "", "Contact name (required)")
	} else {
		cmd.Flags().StringVar(&f.name, "name", "", "Rename the contact")
	}
	cmd.Flags().StringVar(&f.email, "email", "", "Email address")
	cmd.Flags().StringVar(&f.linkedin, "linkedin", "", "LinkedIn profile URL or handle (stored as-is, not validated)")
	cmd.Flags().StringVar(&f.role, "role", "", "Job title or role")
	cmd.Flags().StringVar(&f.company, "company", "", "ID of the company to associate (empty to clear)")
	cmd.Flags().StringVar(&f.notes, "notes", "", "Free-form notes")
}

// newContactCmd is the noun-first parent: `opps contact {create,list,show,update,rm}`.
func newContactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contact",
		Aliases: []string{"contacts"},
		Short:   "Manage contacts",
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newContactCreateCmd(),
		newContactListCmd(),
		newContactShowCmd(),
		newContactUpdateCmd(),
		newContactRmCmd(),
	)
	return cmd
}

func newContactCreateCmd() *cobra.Command {
	var f contactFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a contact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			companyID, err := resolveContactCompany(cmd, svc, f)
			if err != nil {
				return err
			}
			in := service.ContactInput{
				Name:      f.name,
				Email:     f.email,
				LinkedIn:  f.linkedin,
				Role:      f.role,
				CompanyID: companyID,
				Notes:     f.notes,
			}
			c, err := prompt.AddContact(cmd.Context(), svc, in)
			if err != nil {
				return err
			}
			return printContact(cmd.OutOrStdout(), c, asJSON)
		},
	}
	bindContactFlags(cmd, &f, true)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the created contact as JSON")
	return cmd
}

func newContactListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			items, err := svc.ListContacts(cmd.Context())
			if err != nil {
				return err
			}
			return printContacts(cmd.OutOrStdout(), items, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newContactShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show [<id>]",
		Short: "Show a contact",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := resolveContact(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			return printContact(cmd.OutOrStdout(), c, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newContactUpdateCmd() *cobra.Command {
	var f contactFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update [<id>]",
		Short: "Update a contact",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			current, err := resolveContact(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			// Flags().Changed lets us tell "leave alone" (flag absent)
			// from "overwrite with empty" (flag set to ""); without it,
			// both look the same in contactFlags.
			in := contactInputFromModel(current)
			if cmd.Flags().Changed("name") {
				in.Name = f.name
			}
			if cmd.Flags().Changed("email") {
				in.Email = f.email
			}
			if cmd.Flags().Changed("linkedin") {
				in.LinkedIn = f.linkedin
			}
			if cmd.Flags().Changed("role") {
				in.Role = f.role
			}
			if cmd.Flags().Changed("company") {
				in.CompanyID = nullableFlag(f.company)
			}
			if cmd.Flags().Changed("notes") {
				in.Notes = f.notes
			}
			updated, err := svc.UpdateContact(cmd.Context(), current.ID, in)
			if err != nil {
				return err
			}
			return printContact(cmd.OutOrStdout(), updated, asJSON)
		},
	}
	bindContactFlags(cmd, &f, false)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newContactRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm [<id>]",
		Short: "Delete a contact",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := resolveContact(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := prompt.Confirm(cmd.Context(),
					fmt.Sprintf("Delete contact %q (%s)?", c.Name, c.ID))
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("delete aborted")
				}
			}
			return svc.DeleteContact(cmd.Context(), c.ID)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

// resolveContact returns the contact identified by args[0] when present;
// otherwise it picks one via prompt.PickEntity, which auto-selects when
// exactly one contact exists and, in non-interactive mode, requires an
// explicit id only when more than one would be ambiguous.
func resolveContact(ctx context.Context, svc *service.Service, args []string) (model.Contact, error) {
	if len(args) == 1 {
		return svc.GetContact(ctx, args[0])
	}
	items, err := svc.ListContacts(ctx)
	if err != nil {
		return model.Contact{}, err
	}
	return prompt.PickEntity(
		ctx, "Pick a contact", items,
		func(c model.Contact) string { return contactPickLabel(c) },
		func(c model.Contact) string { return c.ID },
	)
}

// resolveContactCompany returns the company id to associate on create. An
// explicit --company flag wins (empty clears to nil); otherwise the user
// picks optionally among existing companies, with "(none)" available.
func resolveContactCompany(cmd *cobra.Command, svc *service.Service, f contactFlags) (*string, error) {
	if cmd.Flags().Changed("company") {
		return nullableFlag(f.company), nil
	}
	companies, err := svc.ListCompanies(cmd.Context())
	if err != nil {
		return nil, err
	}
	c, selected, err := prompt.PickOptional(
		cmd.Context(), "Pick a company (optional)", companies,
		func(c model.Company) string { return fmt.Sprintf("%s (%s)", c.Name, c.Slug) },
		func(c model.Company) string { return c.ID },
	)
	if err != nil {
		return nil, err
	}
	if !selected {
		return nil, nil
	}
	return &c.ID, nil
}

func contactPickLabel(c model.Contact) string {
	if c.CompanyName != nil && *c.CompanyName != "" {
		return fmt.Sprintf("%s — %s", c.Name, *c.CompanyName)
	}
	return c.Name
}

// contactInputFromModel seeds an update with the contact's current values
// so unchanged flags round-trip.
func contactInputFromModel(c model.Contact) service.ContactInput {
	return service.ContactInput{
		Name:      c.Name,
		Email:     c.Email,
		LinkedIn:  c.LinkedIn,
		Role:      c.Role,
		CompanyID: c.CompanyID,
		Notes:     c.Notes,
	}
}

// nullableFlag maps an empty flag string to a nil pointer so an explicit
// `--company=""` clears the association.
func nullableFlag(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// contactCompanyJSON is the nested company object on a contact payload.
type contactCompanyJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// contactJSON shapes the on-the-wire representation. Snake-case keys to
// match the schema and stay stable independent of Go field renames.
type contactJSON struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	Email     string              `json:"email,omitempty"`
	LinkedIn  string              `json:"linkedin,omitempty"`
	Role      string              `json:"role,omitempty"`
	Company   *contactCompanyJSON `json:"company,omitempty"`
	Notes     string              `json:"notes,omitempty"`
	CreatedAt string              `json:"created_at"`
	UpdatedAt string              `json:"updated_at"`
}

func toContactJSON(c model.Contact) contactJSON {
	out := contactJSON{
		ID:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		LinkedIn:  c.LinkedIn,
		Role:      c.Role,
		Notes:     c.Notes,
		CreatedAt: c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if c.CompanyID != nil {
		name := ""
		if c.CompanyName != nil {
			name = *c.CompanyName
		}
		out.Company = &contactCompanyJSON{ID: *c.CompanyID, Name: name}
	}
	return out
}

func printContact(w io.Writer, c model.Contact, asJSON bool) error {
	if asJSON {
		return writeJSON(w, toContactJSON(c))
	}
	company := ""
	if c.CompanyName != nil {
		company = *c.CompanyName
	}
	rows := [][2]string{
		{"ID", c.ID},
		{"Name", oneline(c.Name)},
		{"Email", oneline(c.Email)},
		{"LinkedIn", oneline(c.LinkedIn)},
		{"Role", oneline(c.Role)},
		{"Company", oneline(company)},
		{"Notes", oneline(c.Notes)},
		{"Created", c.CreatedAt.UTC().Format(time.RFC3339)},
		{"Updated", c.UpdatedAt.UTC().Format(time.RFC3339)},
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", r[0], r[1]); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printContacts(w io.Writer, items []model.Contact, asJSON bool) error {
	if asJSON {
		out := make([]contactJSON, len(items))
		for i, c := range items {
			out[i] = toContactJSON(c)
		}
		return writeJSON(w, out)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tROLE\tCOMPANY\tEMAIL"); err != nil {
		return err
	}
	for _, c := range items {
		company := ""
		if c.CompanyName != nil {
			company = *c.CompanyName
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.ID, oneline(c.Name), oneline(c.Role), oneline(company), oneline(c.Email)); err != nil {
			return err
		}
	}
	return tw.Flush()
}
