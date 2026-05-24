package model

import "time"

// Contact is a person tied to a job search — a recruiter, hiring
// manager, referrer, or interviewer. CompanyID is a nullable FK: a
// contact may exist before any company association (e.g. a referrer in
// your network). CompanyName is populated on read via a LEFT JOIN for
// display; it is nil when CompanyID is nil and otherwise mirrors the
// referenced company's name.
type Contact struct {
	ID          string
	Name        string
	Email       string
	LinkedIn    string
	Role        string
	CompanyID   *string
	CompanyName *string
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
