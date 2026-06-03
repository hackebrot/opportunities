package model

import "time"

// OpportunityContact is one row in the opportunity_contacts join table —
// a (contact, relationship) pair attached to an opportunity. The same
// contact can be attached multiple times under different relationships
// (e.g. `recruiter` and later `interviewer`), so the natural key is the
// triple (OpportunityID, ContactID, Relationship), which is also the
// table's primary key.
//
// ContactName is populated on read via a JOIN for display.
type OpportunityContact struct {
	OpportunityID string
	ContactID     string
	ContactName   string
	Relationship  string
	CreatedAt     time.Time
}
