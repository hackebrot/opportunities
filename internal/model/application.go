package model

import "time"

// Application is one attempt at landing a role for a given opportunity. At
// most one application per opportunity can be active at a time
// (applied/in_progress/offer); terminal applications (accepted/rejected/
// declined/withdrawn) free the slot so a re-apply can start a fresh row.
// The active-slot rule is enforced by a partial unique index, not the
// service layer.
//
// AppliedAt is nullable so an application can be captured before the
// exact submission time is known. ArchivedAt and ArchiveReasonCategory
// mirror the terminal event that closed the row (rejected/declined/
// withdrawn → archived; accepted also archives). FollowUpBlocked and
// LastFollowedUpAt drive the staleness picker introduced in T18.
type Application struct {
	ID                    string
	OpportunityID         string
	AppliedAt             *time.Time
	AppliedWithEmail      string
	Status                string
	ArchivedAt            *time.Time
	ArchiveReasonCategory *string
	ArchiveReason         *string
	FollowUpBlocked       bool
	LastFollowedUpAt      *time.Time
	Notes                 string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
