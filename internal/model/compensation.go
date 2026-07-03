package model

import "time"

// Compensation is a salary/equity data point linked to exactly one of an
// opportunity (target/listed ranges captured before applying) or an
// application (offered/counter/accepted figures negotiated after applying)
// — never both, enforced by a DB CHECK. Exactly one of OpportunityID /
// ApplicationID is non-nil.
//
// A parent holds zero or more compensations — nothing constrains the count
// per opportunity or application. An opportunity commonly carries both a
// 'target' (the figure sought) and a 'listed' (the advertised range), and a
// revised figure is recorded as a new row rather than mutating the old one,
// so the most recent row of a given kind is the current one.
//
// Amounts are stored in the currency's minor unit (cents for EUR/USD) as
// integers to avoid float rounding; BonusMinor is nullable when no bonus
// applies. Currency is constrained to 'EUR' or 'USD' by a DB CHECK.
type Compensation struct {
	ID            string
	OpportunityID *string
	ApplicationID *string
	Kind          string
	BaseMinor     int64
	BonusMinor    *int64
	EquityNotes   string
	VestingNotes  string
	Currency      string
	Notes         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
