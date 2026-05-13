package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidLogin = errors.New("invalid username or password")
var ErrInvalidAdmin = errors.New("invalid admin")
var ErrCannotDisableSelf = errors.New("cannot disable the current admin")

const (
	minPasswordLength = 12
	maxPasswordLength = 256
	maxUsernameLength = 64
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type Admin struct {
	ID        int64
	Username  string
	Role      string
	Active    bool
	CreatedAt time.Time
}

func (a Admin) IsOwner() bool {
	return a.Role == RoleOwner
}

func (a Admin) CanManageServers() bool {
	return a.Role == RoleOwner || a.Role == RoleAdmin
}

func (a Admin) CanManageAdmins() bool {
	return a.Role == RoleOwner
}

func (a Admin) CanManageSettings() bool {
	return a.Role == RoleOwner
}

func (a Admin) CanManageLogs() bool {
	return a.Role == RoleOwner || a.Role == RoleAdmin
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Authenticate(ctx context.Context, username, password string) (Admin, error) {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > maxUsernameLength || len(password) > maxPasswordLength {
		return Admin{}, ErrInvalidLogin
	}
	var admin Admin
	var hash string
	err := s.db.QueryRow(ctx, `
		SELECT id, username, password_hash, role, active
		FROM admins
		WHERE username = $1
	`, username).Scan(&admin.ID, &admin.Username, &hash, &admin.Role, &admin.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Admin{}, ErrInvalidLogin
	}
	if err != nil {
		return Admin{}, err
	}
	if !admin.Active || !CheckPassword(hash, password) {
		return Admin{}, ErrInvalidLogin
	}
	return admin, nil
}

func (s *Store) Get(ctx context.Context, id int64) (Admin, error) {
	var admin Admin
	err := s.db.QueryRow(ctx, `
		SELECT id, username, role, active, created_at
		FROM admins
		WHERE id = $1
	`, id).Scan(&admin.ID, &admin.Username, &admin.Role, &admin.Active, &admin.CreatedAt)
	if err != nil {
		return Admin{}, err
	}
	if !admin.Active {
		return Admin{}, ErrInvalidLogin
	}
	return admin, nil
}

func (s *Store) List(ctx context.Context) ([]Admin, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, username, role, active, created_at
		FROM admins
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var admins []Admin
	for rows.Next() {
		var admin Admin
		if err := rows.Scan(&admin.ID, &admin.Username, &admin.Role, &admin.Active, &admin.CreatedAt); err != nil {
			return nil, err
		}
		admins = append(admins, admin)
	}
	return admins, rows.Err()
}

func (s *Store) Create(ctx context.Context, username, password, role string) (Admin, error) {
	username = strings.TrimSpace(username)
	role, ok := normalizeRole(role)
	if !ok {
		return Admin{}, ErrInvalidAdmin
	}
	if err := validateAdminCredentials(username, password); err != nil {
		return Admin{}, ErrInvalidAdmin
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Admin{}, err
	}
	var admin Admin
	err = s.db.QueryRow(ctx, `
		INSERT INTO admins (username, password_hash, role, active)
		VALUES ($1, $2, $3, true)
		RETURNING id, username, role, active, created_at
	`, username, hash, role).Scan(&admin.ID, &admin.Username, &admin.Role, &admin.Active, &admin.CreatedAt)
	if err != nil {
		return Admin{}, err
	}
	return admin, nil
}

func (s *Store) ToggleActive(ctx context.Context, id, currentAdminID int64) (bool, error) {
	if id == currentAdminID {
		return false, ErrCannotDisableSelf
	}
	var active bool
	err := s.db.QueryRow(ctx, `
		UPDATE admins
		SET active = NOT active
		WHERE id = $1
		RETURNING active
	`, id).Scan(&active)
	if err != nil {
		return false, err
	}
	return active, nil
}

func (s *Store) ResetPassword(ctx context.Context, id int64, password string) error {
	if err := validatePassword(password); err != nil {
		return ErrInvalidAdmin
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE admins
		SET password_hash = $1
		WHERE id = $2
	`, hash, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) EnsureInitial(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil
	}

	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM admins`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if err := validateAdminCredentials(username, password); err != nil {
		return fmt.Errorf("invalid initial admin credentials: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO admins (username, password_hash, role, active)
		VALUES ($1, $2, 'owner', true)
	`, username, hash)
	return err
}

func validRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleViewer:
		return true
	default:
		return false
	}
}

func normalizeRole(role string) (string, bool) {
	role = strings.TrimSpace(role)
	if role == "" {
		return RoleAdmin, true
	}
	return role, validRole(role)
}

func validateAdminCredentials(username, password string) error {
	if username == "" || len(username) > maxUsernameLength {
		return ErrInvalidAdmin
	}
	return validatePassword(password)
}

func validatePassword(password string) error {
	if len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return ErrInvalidAdmin
	}
	return nil
}
