package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hackebrot/opportunities/internal/model"
)

// ContactParams is the writable subset of model.Contact used for Create
// and Update. Server-managed columns (id, created_at, updated_at) and
// the read-only CompanyName are not part of it.
type ContactParams struct {
	Name      string
	Email     string
	LinkedIn  string
	Role      string
	CompanyID *string
	Notes     string
}

// contactColumns are the contacts table's own columns, prefixed so they
// can be selected alongside a joined company name without ambiguity.
const contactColumns = `c.id, c.name, c.email, c.linkedin, c.role,
	c.company_id, c.notes, c.created_at, c.updated_at, comp.name`

// CreateContact inserts a contact and returns the persisted row with its
// company name resolved. A company_id that references no company surfaces
// as ErrNotFound.
func (s *Store) CreateContact(ctx context.Context, p ContactParams) (model.Contact, error) {
	const q = `
		WITH ins AS (
			INSERT INTO contacts (name, email, linkedin, role, company_id, notes)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, name, email, linkedin, role, company_id, notes,
				created_at, updated_at
		)
		SELECT c.id, c.name, c.email, c.linkedin, c.role, c.company_id,
			c.notes, c.created_at, c.updated_at, comp.name
		FROM ins c
		LEFT JOIN companies comp ON comp.id = c.company_id`

	row := s.Pool.QueryRow(
		ctx, q,
		p.Name, p.Email, p.LinkedIn, p.Role, p.CompanyID, p.Notes,
	)
	c, err := scanContact(row)
	if err != nil {
		return model.Contact{}, translateContactErr("create", err)
	}
	return c, nil
}

// GetContact returns the contact by id with its company name, or
// ErrNotFound.
func (s *Store) GetContact(ctx context.Context, id string) (model.Contact, error) {
	const q = `SELECT ` + contactColumns + `
		FROM contacts c
		LEFT JOIN companies comp ON comp.id = c.company_id
		WHERE c.id = $1`
	c, err := scanContact(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.Contact{}, translateContactErr("get", err)
	}
	return c, nil
}

// ListContacts returns all contacts ordered by name (case-insensitive),
// each with its company name resolved.
func (s *Store) ListContacts(ctx context.Context) ([]model.Contact, error) {
	const q = `SELECT ` + contactColumns + `
		FROM contacts c
		LEFT JOIN companies comp ON comp.id = c.company_id
		ORDER BY lower(c.name), c.id`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: list contacts: %w", err)
	}
	defer rows.Close()

	var out []model.Contact
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list contacts: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list contacts: %w", err)
	}
	return out, nil
}

// UpdateContact overwrites the writable fields of id and bumps
// updated_at. Returns the post-update row with its company name resolved.
func (s *Store) UpdateContact(ctx context.Context, id string, p ContactParams) (model.Contact, error) {
	const q = `
		WITH upd AS (
			UPDATE contacts
			SET name = $2,
				email = $3,
				linkedin = $4,
				role = $5,
				company_id = $6,
				notes = $7,
				updated_at = now()
			WHERE id = $1
			RETURNING id, name, email, linkedin, role, company_id, notes,
				created_at, updated_at
		)
		SELECT c.id, c.name, c.email, c.linkedin, c.role, c.company_id,
			c.notes, c.created_at, c.updated_at, comp.name
		FROM upd c
		LEFT JOIN companies comp ON comp.id = c.company_id`

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.Name, p.Email, p.LinkedIn, p.Role, p.CompanyID, p.Notes,
	)
	c, err := scanContact(row)
	if err != nil {
		return model.Contact{}, translateContactErr("update", err)
	}
	return c, nil
}

// DeleteContact removes the contact by id. Missing id is ErrNotFound.
func (s *Store) DeleteContact(ctx context.Context, id string) error {
	const q = `DELETE FROM contacts WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete contact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanContact(r rowScanner) (model.Contact, error) {
	var c model.Contact
	err := r.Scan(
		&c.ID, &c.Name, &c.Email, &c.LinkedIn, &c.Role,
		&c.CompanyID, &c.Notes, &c.CreatedAt, &c.UpdatedAt, &c.CompanyName,
	)
	return c, err
}

func translateContactErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == pgForeignKeyViolation {
		// company_id pointed at a company that doesn't exist. Name the
		// company so the caller isn't left guessing which entity is missing.
		return fmt.Errorf("%w: unknown company ID", ErrNotFound)
	}
	return fmt.Errorf("store: %s contact: %w", op, err)
}
