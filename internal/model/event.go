package model

import "time"

// Event is one entry in an opportunity's timeline. OpportunityID is always
// set; ApplicationID is set once an application exists, and StageID once a
// stage is linked (both nullable). Label is only meaningful when Kind is
// "custom". OccurredAt is when the event happened (service-supplied via the
// injected clock); CreatedAt is when the row was written.
type Event struct {
	ID            string
	OpportunityID string
	ApplicationID *string
	StageID       *string
	Kind          string
	OccurredAt    time.Time
	Label         *string
	Notes         string
	CreatedAt     time.Time
}
