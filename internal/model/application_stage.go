package model

import "time"

// ApplicationStage is one step in an application's interview pipeline: an
// ordered, editable list of stages — as advertised by the company's
// applicant-tracking system, or inferred during the process — each with
// expectation and prep notes. OrderIndex is unique per application and
// drives display order; ScheduledAt, CompletedAt, and Outcome are nullable
// because a stage can be listed before it is scheduled and its outcome
// ('passed'|'failed'|'skipped') is unknown until it completes.
type ApplicationStage struct {
	ID               string
	ApplicationID    string
	OrderIndex       int
	Label            string
	ExpectationNotes string
	PrepNotes        string
	ScheduledAt      *time.Time
	CompletedAt      *time.Time
	Outcome          *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
