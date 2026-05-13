package aijson

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) List(ctx context.Context, activeOnly bool) ([]Method, error) {
	where := ""
	if activeOnly {
		where = "WHERE m.active = true"
	}
	rows, err := s.db.Query(ctx, `
		SELECT m.id, m.name, m.description, m.source, coalesce(m.event_type, ''), coalesce(m.resource, ''),
			m.prompt, m.spec, m.active, m.created_by, coalesce(a.username, ''),
			m.created_at, m.updated_at
		FROM ai_json_methods m
		LEFT JOIN admins a ON a.id = m.created_by
		`+where+`
		ORDER BY m.active DESC, m.updated_at DESC, m.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var methods []Method
	for rows.Next() {
		method, err := scanMethod(rows)
		if err != nil {
			return nil, err
		}
		methods = append(methods, method)
	}
	return methods, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Method, error) {
	var row methodScanner = s.db.QueryRow(ctx, `
		SELECT m.id, m.name, m.description, m.source, coalesce(m.event_type, ''), coalesce(m.resource, ''),
			m.prompt, m.spec, m.active, m.created_by, coalesce(a.username, ''),
			m.created_at, m.updated_at
		FROM ai_json_methods m
		LEFT JOIN admins a ON a.id = m.created_by
		WHERE m.id = $1
	`, id)
	return scanMethod(row)
}

func (s *Store) Create(ctx context.Context, input MethodInput, adminID int64) (Method, error) {
	input, err := NormalizeInput(input)
	if err != nil {
		return Method{}, err
	}
	var row methodScanner = s.db.QueryRow(ctx, `
		INSERT INTO ai_json_methods (
			name, description, source, event_type, resource, prompt, spec, active, created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, true, $8)
		RETURNING id, name, description, source, coalesce(event_type, ''), coalesce(resource, ''), prompt, spec,
			active, created_by, '', created_at, updated_at
	`, input.Name, input.Description, input.Source, nullable(input.EventType), nullable(input.Resource), input.Prompt, input.Spec, nullableInt64(adminID))
	return scanMethod(row)
}

func (s *Store) Update(ctx context.Context, id int64, input MethodInput) (Method, error) {
	if id <= 0 {
		return Method{}, ErrInvalidMethod
	}
	input, err := NormalizeInput(input)
	if err != nil {
		return Method{}, err
	}
	var row methodScanner = s.db.QueryRow(ctx, `
		UPDATE ai_json_methods
		SET name = $2,
			description = $3,
			source = $4,
			event_type = $5,
			resource = $6,
			prompt = $7,
			spec = $8::jsonb,
			updated_at = now()
		WHERE id = $1
		RETURNING id, name, description, source, coalesce(event_type, ''), coalesce(resource, ''), prompt, spec,
			active, created_by, '', created_at, updated_at
	`, id, input.Name, input.Description, input.Source, nullable(input.EventType), nullable(input.Resource), input.Prompt, input.Spec)
	return scanMethod(row)
}

func (s *Store) ToggleActive(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, ErrInvalidMethod
	}
	var active bool
	err := s.db.QueryRow(ctx, `
		UPDATE ai_json_methods
		SET active = NOT active, updated_at = now()
		WHERE id = $1
		RETURNING active
	`, id).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrInvalidMethod
	}
	return active, err
}

type methodScanner interface {
	Scan(...any) error
}

func scanMethod(row methodScanner) (Method, error) {
	var method Method
	var createdBy *int64
	if err := row.Scan(&method.ID, &method.Name, &method.Description, &method.Source,
		&method.EventType, &method.Resource, &method.Prompt, &method.Spec, &method.Active,
		&createdBy, &method.CreatedByName, &method.CreatedAt, &method.UpdatedAt); err != nil {
		return Method{}, err
	}
	method.CreatedBy = createdBy
	if len(method.Spec) == 0 || !json.Valid(method.Spec) {
		method.Spec = json.RawMessage(`{}`)
	}
	return method, nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}
