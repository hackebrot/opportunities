package model

import "time"

// Company is a potential employer. Slug is derived from Name by the
// service layer (lowercase, alphanumeric only) and is treated as
// immutable once set unless the company is renamed.
type Company struct {
	ID             string
	Name           string
	Slug           string
	Website        string
	CareersURL     string
	PreferredEmail *string
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
