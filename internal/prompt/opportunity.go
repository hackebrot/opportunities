package prompt

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// OpportunityCreator is the subset of *service.Service needed by
// AddOpportunity. Defined as an interface so tests can substitute a fake
// without standing up a real store.
type OpportunityCreator interface {
	ListCompanies(ctx context.Context) ([]model.Company, error)
	ListContacts(ctx context.Context) ([]model.Contact, error)
	AddOpportunity(ctx context.Context, in service.OpportunityCreationInput) (model.Opportunity, error)
}

// OfficeDaysUnset is a UI-layer sentinel meaning "the user didn't supply
// a value." 0 is a valid choice (remote) so Go's int zero value can't
// double as "absent." The CLI defaults its --office-days-per-week flag
// to this constant; promptOpportunityBody fires a Select menu when it
// sees this value interactively, and returns ErrNonInteractive when it
// sees it non-interactively — same shape as the source/priority pickers.
//
// Invariant: this sentinel never crosses the prompt/service boundary.
// AddOpportunity either replaces it with a real 0..5 (interactive path)
// or returns ErrNonInteractive before calling the service, which stays
// strict 0..5.
const OfficeDaysUnset = -1

// opportunitySources is the ordered display list for the "source" picker.
// The slice owns the canonical order; the map mirrors validSources in the
// service layer but the service is the source of truth — this is purely
// UX.
var opportunitySources = []Option{
	{Key: "outbound", Label: "Outbound — I applied / reached out"},
	{Key: "inbound_inhouse_recruiter", Label: "Inbound — in-house recruiter"},
	{Key: "inbound_external_recruiter", Label: "Inbound — external recruiter"},
	{Key: "inbound_founder", Label: "Inbound — founder"},
	{Key: "inbound_employee", Label: "Inbound — employee"},
	{Key: "referral", Label: "Referral"},
	{Key: "network", Label: "Network"},
	{Key: "other", Label: "Other"},
}

var opportunityPriorities = []Option{
	{Key: "normal", Label: "Normal"},
	{Key: "low", Label: "Low"},
	{Key: "high", Label: "High"},
}

var opportunityRelationships = []Option{
	{Key: "recruiter", Label: "Recruiter"},
	{Key: "hiring_manager", Label: "Hiring manager"},
	{Key: "referrer", Label: "Referrer"},
	{Key: "interviewer", Label: "Interviewer"},
	{Key: "other", Label: "Other"},
}

// opportunityOfficeDays is the menu the office-days picker offers. Keys
// are the integer 0..5 stringified so the picker's generic Option type
// can carry them; the prompt converts back to int after Select returns.
var opportunityOfficeDays = []Option{
	{Key: "0", Label: "Remote"},
	{Key: "1", Label: "Hybrid (1 day in office)"},
	{Key: "2", Label: "Hybrid (2 days in office)"},
	{Key: "3", Label: "Hybrid (3 days in office)"},
	{Key: "4", Label: "Hybrid (4 days in office)"},
	{Key: "5", Label: "Onsite"},
}

// AddOpportunity is the reusable opportunity-creation flow. Fields
// already set in prefill.Opportunity are trusted as-is (callers own
// flag validation); the rest are prompted interactively. The company is
// picked or created inline via PickOrCreate, and an optional contact
// can be attached the same way. The whole graph commits atomically
// through service.AddOpportunity.
//
// In non-interactive mode the caller must supply at minimum
// prefill.Company.ID (or prefill.Company.New) and prefill.Opportunity.Source.
func AddOpportunity(ctx context.Context, c OpportunityCreator, prefill service.OpportunityCreationInput) (model.Opportunity, error) {
	in := prefill

	// Step 1: company. pickOrCaptureNewCompany shows an interactive
	// picker with a "[+ New …]" branch for inline creation; in
	// non-interactive mode it returns ErrNonInteractive unconditionally.
	// Unlike the read-class helpers (resolveCompany, resolveContact) it
	// does not auto-select when exactly one company exists — creates
	// establish a permanent association, so the caller must pick
	// deliberately via --company.
	if in.Company.ID == "" && in.Company.New == nil {
		if err := resolveCompanyChoice(ctx, c, &in.Company); err != nil {
			return model.Opportunity{}, err
		}
	}

	// Step 2: opportunity body — text fields and contextual menus.
	if err := promptOpportunityBody(ctx, &in.Opportunity); err != nil {
		return model.Opportunity{}, err
	}

	// Step 3: contact attachment is optional. If the caller pre-filled
	// it (via flags), respect that and skip the "add a contact?" prompt;
	// otherwise ask, and only descend into picker/new-contact if yes.
	if in.Contact == nil && !IsNonInteractive(ctx) {
		add, err := InterfaceFrom(ctx).Confirm("Add a contact for this opportunity?")
		if err != nil {
			return model.Opportunity{}, err
		}
		if add {
			contact, err := resolveContactChoice(ctx, c, in.Company)
			if err != nil {
				return model.Opportunity{}, err
			}
			in.Contact = contact
		}
	}

	return c.AddOpportunity(ctx, in)
}

// resolveCompanyChoice prompts the user to pick an existing company or
// capture a new one for inline creation in the same tx. The new branch
// returns a CompanyInput rather than persisting — service writes happen
// later inside the bundled transaction.
func resolveCompanyChoice(ctx context.Context, c OpportunityCreator, choice *service.OpportunityCompanyChoice) error {
	companies, err := c.ListCompanies(ctx)
	if err != nil {
		return err
	}
	id, newCompany, err := pickOrCaptureNewCompany(ctx, companies)
	if err != nil {
		return err
	}
	if newCompany != nil {
		choice.New = newCompany
	} else {
		choice.ID = id
	}
	return nil
}

// pickOrCaptureNewCompany is the company picker for the inline-create
// flow: returns either an existing id or a CompanyInput to be inserted
// in the caller's transaction. In non-interactive mode it fails with
// ErrNonInteractive — unlike the read-class helpers (resolveCompany,
// resolveContact) which auto-select when exactly one row exists,
// "create an opportunity" establishes a permanent association, so the
// caller must pick the company explicitly via --company rather than
// letting the CLI silently fall through to "the only one in the DB".
func pickOrCaptureNewCompany(ctx context.Context, companies []model.Company) (id string, newCompany *service.CompanyInput, err error) {
	if IsNonInteractive(ctx) {
		return "", nil, fmt.Errorf("%w: company is required", ErrNonInteractive)
	}
	opts := make([]Option, 0, len(companies)+1)
	opts = append(opts, Option{Key: NewItemKey, Label: "[+ New company]"})
	for _, c := range companies {
		opts = append(opts, Option{Key: c.ID, Label: fmt.Sprintf("%s (%s)", c.Name, c.Slug)})
	}
	k, err := InterfaceFrom(ctx).Select("Pick a company", opts)
	if err != nil {
		return "", nil, err
	}
	if k == NewItemKey {
		var input service.CompanyInput
		if err := captureCompanyInput(ctx, &input); err != nil {
			return "", nil, err
		}
		return "", &input, nil
	}
	return k, nil, nil
}

// captureCompanyInput mirrors AddCompany's prompts but only records
// values into in — the actual insert is deferred to the bundled tx.
func captureCompanyInput(ctx context.Context, in *service.CompanyInput) error {
	if err := Text(ctx, "Company name", &in.Name); err != nil {
		return err
	}
	if err := textOptional(ctx, "Website (optional)", &in.Website); err != nil {
		return err
	}
	if err := textOptional(ctx, "Careers URL (optional)", &in.CareersURL); err != nil {
		return err
	}
	if err := textOptional(ctx, "Preferred email (optional)", &in.PreferredEmail); err != nil {
		return err
	}
	return textOptional(ctx, "Notes (optional)", &in.Notes)
}

func captureContactInput(ctx context.Context, in *service.ContactInput) error {
	if err := Text(ctx, "Contact name", &in.Name); err != nil {
		return err
	}
	if err := textOptional(ctx, "Email (optional)", &in.Email); err != nil {
		return err
	}
	if err := textOptional(ctx, "LinkedIn (optional)", &in.LinkedIn); err != nil {
		return err
	}
	if err := textOptional(ctx, "Role (optional)", &in.Role); err != nil {
		return err
	}
	return textOptional(ctx, "Notes (optional)", &in.Notes)
}

func resolveContactChoice(ctx context.Context, c OpportunityCreator, company service.OpportunityCompanyChoice) (*service.OpportunityContactChoice, error) {
	contacts, err := c.ListContacts(ctx)
	if err != nil {
		return nil, err
	}
	id, newContact, err := pickOrCaptureNewContact(ctx, contacts)
	if err != nil {
		return nil, err
	}
	relationship, err := pickRelationship(ctx)
	if err != nil {
		return nil, err
	}
	choice := &service.OpportunityContactChoice{Relationship: relationship}
	if newContact != nil {
		// Pre-populate the company id so an inline-created contact lands
		// under the resolved company. When the company itself is also
		// new (no id yet), leave CompanyID nil — the service will fill
		// it in once the company insert returns.
		if company.ID != "" {
			cid := company.ID
			newContact.CompanyID = &cid
		}
		choice.New = newContact
	} else {
		choice.ID = id
	}
	return choice, nil
}

func pickOrCaptureNewContact(ctx context.Context, contacts []model.Contact) (id string, newContact *service.ContactInput, err error) {
	if IsNonInteractive(ctx) {
		// Should not be reached: the parent already short-circuited
		// without asking. Defensive against future refactors.
		return "", nil, ErrNonInteractive
	}
	opts := make([]Option, 0, len(contacts)+1)
	opts = append(opts, Option{Key: NewItemKey, Label: "[+ New contact]"})
	for _, c := range contacts {
		opts = append(opts, Option{Key: c.ID, Label: contactPickLabel(c)})
	}
	k, err := InterfaceFrom(ctx).Select("Pick a contact", opts)
	if err != nil {
		return "", nil, err
	}
	if k == NewItemKey {
		var input service.ContactInput
		if err := captureContactInput(ctx, &input); err != nil {
			return "", nil, err
		}
		return "", &input, nil
	}
	return k, nil, nil
}

func contactPickLabel(c model.Contact) string {
	if c.CompanyName != nil && *c.CompanyName != "" {
		return fmt.Sprintf("%s (%s)", c.Name, *c.CompanyName)
	}
	return c.Name
}

func pickRelationship(ctx context.Context) (string, error) {
	if IsNonInteractive(ctx) {
		return "", fmt.Errorf("%w: relationship is required", ErrNonInteractive)
	}
	return InterfaceFrom(ctx).Select("Relationship", opportunityRelationships)
}

// promptOpportunityBody captures the opportunity's own fields.
// Non-interactive callers must supply Source and OfficeDaysPerWeek
// (both error with ErrNonInteractive when missing); Priority falls
// through to the service's "normal" default if left blank; role
// title, location, source detail, and notes are pure optionals that
// stay empty when not prefilled.
func promptOpportunityBody(ctx context.Context, in *service.OpportunityInput) error {
	if err := textOptional(ctx, "Role title (optional)", &in.RoleTitle); err != nil {
		return err
	}
	if err := textOptional(ctx, "Location (optional)", &in.Location); err != nil {
		return err
	}
	if in.OfficeDaysPerWeek == OfficeDaysUnset {
		days, err := pickOfficeDays(ctx)
		if err != nil {
			return err
		}
		in.OfficeDaysPerWeek = days
	}
	if in.Source == "" {
		src, err := pickFromMenu(ctx, "Source", opportunitySources)
		if err != nil {
			return err
		}
		in.Source = src
	}
	if err := textOptional(ctx, "Source detail (optional)", &in.SourceDetail); err != nil {
		return err
	}
	// Priority falls through to the service's default ("normal") in
	// non-interactive mode; only prompt interactively.
	if in.Priority == "" && !IsNonInteractive(ctx) {
		p, err := pickFromMenu(ctx, "Priority", opportunityPriorities)
		if err != nil {
			return err
		}
		in.Priority = p
	}
	return textOptional(ctx, "Notes (optional)", &in.Notes)
}

// pickOfficeDays presents the menu of 0..5 days and returns the chosen
// integer. Treated as required (no default) — non-interactive callers
// without the flag get ErrNonInteractive, matching how the source and
// priority menus behave.
func pickOfficeDays(ctx context.Context) (int, error) {
	key, err := pickFromMenu(ctx, "Office days per week", opportunityOfficeDays)
	if err != nil {
		return 0, err
	}
	// pickFromMenu only returns keys that came from opportunityOfficeDays,
	// every one of which is a valid 0..5 string literal — Atoi cannot
	// fail here unless the menu definition is corrupted at compile time.
	n, err := strconv.Atoi(key)
	if err != nil {
		return 0, fmt.Errorf("office days picker returned non-integer key %q: %w", key, err)
	}
	return n, nil
}

// pickFromMenu is a thin wrapper around Interface.Select used for the
// fixed enum-style menus (source, priority). Non-interactive callers
// must pre-supply the value; an empty input here means no flag was set.
func pickFromMenu(ctx context.Context, title string, opts []Option) (string, error) {
	if IsNonInteractive(ctx) {
		return "", fmt.Errorf("%w: %s is required", ErrNonInteractive, title)
	}
	return InterfaceFrom(ctx).Select(title, opts)
}
