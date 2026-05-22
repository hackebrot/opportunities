package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

// companyFlags mirrors service.CompanyInput. On update, cmd.Flags().Changed
// is the source of truth for "user supplied this flag" — both omitted
// flags and explicit `--foo=""` deserialize to the empty string here.
type companyFlags struct {
	name           string
	website        string
	careersURL     string
	preferredEmail string
	notes          string
}

func bindCompanyFlags(cmd *cobra.Command, f *companyFlags, includeName bool) {
	if includeName {
		cmd.Flags().StringVar(&f.name, "name", "", "Company name (required)")
	} else {
		cmd.Flags().StringVar(&f.name, "name", "", "Rename the company")
	}
	cmd.Flags().StringVar(&f.website, "website", "", "Marketing site URL")
	cmd.Flags().StringVar(&f.careersURL, "careers-url", "", "Careers page URL")
	cmd.Flags().StringVar(&f.preferredEmail, "preferred-email", "", "Your reply-to address for applications to this company (overrides the templated default)")
	cmd.Flags().StringVar(&f.notes, "notes", "", "Free-form notes")
}

// newCompanyCmd is the noun-first parent: `opps company {create,list,show,update,rm}`.
// One group per entity, CRUD verbs as children — future entities
// (contact, opportunity, application) follow the same template.
func newCompanyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "company",
		Aliases: []string{"companies"},
		Short:   "Manage companies",
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newCompanyCreateCmd(),
		newCompanyListCmd(),
		newCompanyShowCmd(),
		newCompanyUpdateCmd(),
		newCompanyRmCmd(),
	)
	return cmd
}

func newCompanyCreateCmd() *cobra.Command {
	var f companyFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a company",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			in := service.CompanyInput{
				Name:           f.name,
				Website:        f.website,
				CareersURL:     f.careersURL,
				PreferredEmail: f.preferredEmail,
				Notes:          f.notes,
			}
			c, err := prompt.AddCompany(cmd.Context(), svc, in)
			if err != nil {
				return err
			}
			return printCompany(cmd.OutOrStdout(), c, asJSON)
		},
	}
	bindCompanyFlags(cmd, &f, true)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the created company as JSON")
	return cmd
}

func newCompanyListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List companies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			items, err := svc.ListCompanies(cmd.Context())
			if err != nil {
				return err
			}
			return printCompanies(cmd.OutOrStdout(), items, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newCompanyShowCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show [<id>]",
		Short: "Show a company",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := resolveCompany(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			return printCompany(cmd.OutOrStdout(), c, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newCompanyUpdateCmd() *cobra.Command {
	var f companyFlags
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update [<id>]",
		Short: "Update a company",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			current, err := resolveCompany(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			// Flags().Changed lets us tell "leave alone" (flag absent)
			// from "overwrite with empty" (flag set to ""); without it,
			// both look the same in companyFlags.
			in := companyInputFromModel(current)
			if cmd.Flags().Changed("name") {
				in.Name = f.name
			}
			if cmd.Flags().Changed("website") {
				in.Website = f.website
			}
			if cmd.Flags().Changed("careers-url") {
				in.CareersURL = f.careersURL
			}
			if cmd.Flags().Changed("preferred-email") {
				in.PreferredEmail = f.preferredEmail
			}
			if cmd.Flags().Changed("notes") {
				in.Notes = f.notes
			}
			updated, err := svc.UpdateCompany(cmd.Context(), current.ID, in)
			if err != nil {
				return err
			}
			return printCompany(cmd.OutOrStdout(), updated, asJSON)
		},
	}
	bindCompanyFlags(cmd, &f, false)
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON instead of a table")
	return cmd
}

func newCompanyRmCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rm [<id>]",
		Short: "Delete a company",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := openServiceFromConfig(cmd)
			if err != nil {
				return err
			}
			defer closeFn()
			c, err := resolveCompany(cmd.Context(), svc, args)
			if err != nil {
				return err
			}
			if !yes {
				ok, err := prompt.Confirm(cmd.Context(),
					fmt.Sprintf("Delete company %q (%s)?", c.Name, c.ID))
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("delete aborted")
				}
			}
			return svc.DeleteCompany(cmd.Context(), c.ID)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Skip the interactive confirmation")
	return cmd
}

// resolveCompany returns the company identified by args[0] when present;
// otherwise it prompts the user to pick one. In non-interactive mode an
// explicit id is required.
func resolveCompany(ctx context.Context, svc *service.Service, args []string) (model.Company, error) {
	if len(args) == 1 {
		return svc.GetCompany(ctx, args[0])
	}
	items, err := svc.ListCompanies(ctx)
	if err != nil {
		return model.Company{}, err
	}
	return prompt.PickEntity(
		ctx, "Pick a company", items,
		func(c model.Company) string { return fmt.Sprintf("%s (%s)", c.Name, c.Slug) },
		func(c model.Company) string { return c.ID },
	)
}

// companyInputFromModel seeds an update with the company's current
// values so unchanged flags round-trip. Slug is intentionally omitted —
// the service re-derives it from Name on every UpdateCompany.
func companyInputFromModel(c model.Company) service.CompanyInput {
	in := service.CompanyInput{
		Name:       c.Name,
		Website:    c.Website,
		CareersURL: c.CareersURL,
		Notes:      c.Notes,
	}
	if c.PreferredEmail != nil {
		in.PreferredEmail = *c.PreferredEmail
	}
	return in
}

// companyJSON shapes the on-the-wire representation. Snake-case keys to
// match the schema and stay stable independent of Go field renames.
type companyJSON struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Website        string  `json:"website,omitempty"`
	CareersURL     string  `json:"careers_url,omitempty"`
	PreferredEmail *string `json:"preferred_email,omitempty"`
	Notes          string  `json:"notes,omitempty"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func toCompanyJSON(c model.Company) companyJSON {
	return companyJSON{
		ID:             c.ID,
		Name:           c.Name,
		Slug:           c.Slug,
		Website:        c.Website,
		CareersURL:     c.CareersURL,
		PreferredEmail: c.PreferredEmail,
		Notes:          c.Notes,
		CreatedAt:      c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func printCompany(w io.Writer, c model.Company, asJSON bool) error {
	if asJSON {
		return writeJSON(w, toCompanyJSON(c))
	}
	email := ""
	if c.PreferredEmail != nil {
		email = *c.PreferredEmail
	}
	rows := [][2]string{
		{"ID", c.ID},
		{"Name", oneline(c.Name)},
		{"Slug", c.Slug},
		{"Website", oneline(c.Website)},
		{"Careers URL", oneline(c.CareersURL)},
		{"Preferred email", oneline(email)},
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

// oneline collapses newlines to spaces so a multi-line field can't break
// the tabwriter's column alignment.
func oneline(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func printCompanies(w io.Writer, items []model.Company, asJSON bool) error {
	if asJSON {
		out := make([]companyJSON, len(items))
		for i, c := range items {
			out[i] = toCompanyJSON(c)
		}
		return writeJSON(w, out)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tSLUG\tWEBSITE"); err != nil {
		return err
	}
	for _, c := range items {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", c.ID, oneline(c.Name), c.Slug, oneline(c.Website)); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// openServiceFromConfig is the service-layer twin of openStoreFromConfig.
// Returns a close func the caller defers; the func releases the
// underlying pool.
func openServiceFromConfig(cmd *cobra.Command) (*service.Service, func(), error) {
	s, err := openStoreFromConfig(cmd)
	if err != nil {
		return nil, nil, err
	}
	return service.New(s), s.Close, nil
}
