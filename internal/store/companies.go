package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hackebrot/opportunities/internal/model"
)

// CompanyParams is the writable subset of model.Company used for
// Create and Update. Server-managed columns (id, created_at,
// updated_at) are not part of it.
type CompanyParams struct {
	Name           string
	Slug           string
	Website        string
	CareersURL     string
	PreferredEmail *string
	Notes          string
}

const companyColumns = `id, name, slug, website, careers_url, preferred_email, notes, created_at, updated_at`

// CreateCompany inserts a company and returns the persisted row.
// Duplicate slug surfaces as ErrConflict.
func (s *Store) CreateCompany(ctx context.Context, p CompanyParams) (model.Company, error) {
	const q = `
		INSERT INTO companies (name, slug, website, careers_url,
			preferred_email, notes)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + companyColumns

	row := s.Pool.QueryRow(
		ctx, q,
		p.Name, p.Slug, p.Website, p.CareersURL, p.PreferredEmail, p.Notes,
	)
	c, err := scanCompany(row)
	if err != nil {
		return model.Company{}, translateCompanyErr("create", err)
	}
	return c, nil
}

// GetCompany returns the company by id, or ErrNotFound.
func (s *Store) GetCompany(ctx context.Context, id string) (model.Company, error) {
	const q = `SELECT ` + companyColumns + ` FROM companies WHERE id = $1`
	c, err := scanCompany(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.Company{}, translateCompanyErr("get", err)
	}
	return c, nil
}

// ListCompanies returns all companies ordered by name (case-insensitive).
func (s *Store) ListCompanies(ctx context.Context) ([]model.Company, error) {
	const q = `SELECT ` + companyColumns + `
		FROM companies
		ORDER BY lower(name), id`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: list companies: %w", err)
	}
	defer rows.Close()

	var out []model.Company
	for rows.Next() {
		c, err := scanCompany(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list companies: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list companies: %w", err)
	}
	return out, nil
}

// UpdateCompany overwrites the writable fields of id and bumps
// updated_at. Returns the post-update row.
func (s *Store) UpdateCompany(ctx context.Context, id string, p CompanyParams) (model.Company, error) {
	const q = `
		UPDATE companies
		SET name = $2,
			slug = $3,
			website = $4,
			careers_url = $5,
			preferred_email = $6,
			notes = $7,
			updated_at = now()
		WHERE id = $1
		RETURNING ` + companyColumns

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.Name, p.Slug, p.Website, p.CareersURL, p.PreferredEmail, p.Notes,
	)
	c, err := scanCompany(row)
	if err != nil {
		return model.Company{}, translateCompanyErr("update", err)
	}
	return c, nil
}

// DeleteCompany removes the company by id. Missing id is ErrNotFound.
func (s *Store) DeleteCompany(ctx context.Context, id string) error {
	const q = `DELETE FROM companies WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete company: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCompany(r rowScanner) (model.Company, error) {
	var c model.Company
	err := r.Scan(
		&c.ID, &c.Name, &c.Slug, &c.Website, &c.CareersURL,
		&c.PreferredEmail, &c.Notes, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func translateCompanyErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == pgUniqueViolation {
		return ErrConflict
	}
	return fmt.Errorf("store: %s company: %w", op, err)
}
