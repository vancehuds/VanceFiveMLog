package serverkeys

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidAPIKey = errors.New("invalid api key")
var ErrInvalidServerName = errors.New("server name is required")

const maxServerNameLength = 80
const eventMarkThrottleSeconds = 5

type Server struct {
	ID          int64
	Name        string
	Active      bool
	CreatedAt   time.Time
	RotatedAt   *time.Time
	LastSeenAt  *time.Time
	LastEventAt *time.Time
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, name string) (Server, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxServerNameLength {
		return Server{}, "", ErrInvalidServerName
	}
	key, hash, err := NewAPIKey()
	if err != nil {
		return Server{}, "", err
	}
	var server Server
	err = s.db.QueryRow(ctx, `
		INSERT INTO servers (name, api_key_hash, active)
		VALUES ($1, $2, true)
		RETURNING id, name, active, created_at, rotated_at, last_seen_at, last_event_at
	`, name, hash).Scan(&server.ID, &server.Name, &server.Active, &server.CreatedAt, &server.RotatedAt, &server.LastSeenAt, &server.LastEventAt)
	if err != nil {
		return Server{}, "", err
	}
	return server, key, nil
}

func (s *Store) ToggleActive(ctx context.Context, id int64) (bool, error) {
	var active bool
	err := s.db.QueryRow(ctx, `
		UPDATE servers
		SET active = NOT active
		WHERE id = $1
		RETURNING active
	`, id).Scan(&active)
	if err != nil {
		return false, err
	}
	return active, nil
}

func (s *Store) RotateKey(ctx context.Context, id int64) (string, error) {
	key, hash, err := NewAPIKey()
	if err != nil {
		return "", err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE servers
		SET api_key_hash = $1, rotated_at = now()
		WHERE id = $2
	`, hash, id)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", pgx.ErrNoRows
	}
	return key, nil
}

func (s *Store) List(ctx context.Context) ([]Server, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, active, created_at, rotated_at, last_seen_at, last_event_at
		FROM servers
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var server Server
		if err := rows.Scan(&server.ID, &server.Name, &server.Active, &server.CreatedAt, &server.RotatedAt, &server.LastSeenAt, &server.LastEventAt); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	return servers, rows.Err()
}

func (s *Store) Authenticate(ctx context.Context, key string) (Server, error) {
	key = strings.TrimSpace(strings.TrimPrefix(key, "Bearer "))
	if key == "" {
		return Server{}, ErrInvalidAPIKey
	}

	candidate := HashAPIKey(key)
	var server Server
	err := s.db.QueryRow(ctx, `
		SELECT id, name, active, created_at, rotated_at, last_seen_at, last_event_at
		FROM servers
		WHERE api_key_hash = $1 AND active = true
	`, candidate).Scan(&server.ID, &server.Name, &server.Active, &server.CreatedAt, &server.RotatedAt, &server.LastSeenAt, &server.LastEventAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Server{}, ErrInvalidAPIKey
	}
	return server, err
}

func (s *Store) MarkSeen(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE servers
		SET last_seen_at = now()
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MarkEvent(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE servers
		SET last_seen_at = now(), last_event_at = now()
		WHERE id = $1
			AND (last_event_at IS NULL OR last_event_at < now() - ($2::int * interval '1 second'))
	`, id, eventMarkThrottleSeconds)
	if err != nil {
		return err
	}
	_ = tag
	return nil
}

func NewAPIKey() (plain string, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = "vfl_" + base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashAPIKey(plain), nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return fmt.Sprintf("sha256:%x", sum[:])
}
