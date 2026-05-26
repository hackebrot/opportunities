package model

import "time"

// Opportunity is a role being tracked at a company, from initial interest
// through to an application. CompanyName is populated on read via a JOIN
// for display; the FK (CompanyID) is required and never null.
//
// RoleTitle is nullable so an opportunity can be captured before the
// exact role is known. OfficeDaysPerWeek is 0..5: 0 → remote, 5 → onsite,
// 1..4 → hybrid (N/5); the display label is derived, only the integer is
// stored. LatestStatus is a denormalized mirror of the opportunity's
// current state, maintained solely by the service layer.
type Opportunity struct {
	ID                string
	CompanyID         string
	CompanyName       string
	RoleTitle         *string
	Location          string
	OfficeDaysPerWeek int
	Source            string
	SourceDetail      string
	Priority          string
	LatestStatus      string
	ArchivedAt        *time.Time
	ArchiveReason     *string
	Notes             string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
