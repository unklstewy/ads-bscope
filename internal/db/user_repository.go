// Package db provides database access for ADS-B Scope
package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// User represents a user account in the system
type User struct {
	ID            int       `json:"id"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	PasswordHash  string    `json:"-"` // Never expose password hash in JSON
	Role          string    `json:"role"`
	IsActive      bool      `json:"is_active"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastLogin     *time.Time `json:"last_login,omitempty"`
}

var (
	// ErrUserNotFound is returned when a user cannot be found
	ErrUserNotFound = errors.New("user not found")
	// ErrUserExists is returned when trying to create a user that already exists
	ErrUserExists = errors.New("user already exists")
)

// UserRepository provides methods for user database operations
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user in the database
func (r *UserRepository) Create(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (username, email, password_hash, role, is_active, email_verified)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`
	
	err := r.db.QueryRowContext(
		ctx,
		query,
		user.Username,
		user.Email,
		user.PasswordHash,
		user.Role,
		user.IsActive,
		user.EmailVerified,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	
	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return ErrUserExists
		}
		return err
	}
	
	return nil
}

// GetByID retrieves a user by their ID
func (r *UserRepository) GetByID(ctx context.Context, id int) (*User, error) {
	query := `
		SELECT id, username, email, password_hash, role, is_active, email_verified,
		       created_at, updated_at, last_login
		FROM users
		WHERE id = $1
	`
	
	user := &User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.IsActive,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLogin,
	)
	
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	
	return user, nil
}

// GetByUsername retrieves a user by their username
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT id, username, email, password_hash, role, is_active, email_verified,
		       created_at, updated_at, last_login
		FROM users
		WHERE username = $1
	`
	
	user := &User{}
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.IsActive,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLogin,
	)
	
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	
	return user, nil
}

// GetByEmail retrieves a user by their email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT id, username, email, password_hash, role, is_active, email_verified,
		       created_at, updated_at, last_login
		FROM users
		WHERE email = $1
	`
	
	user := &User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.PasswordHash,
		&user.Role,
		&user.IsActive,
		&user.EmailVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLogin,
	)
	
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	
	return user, nil
}

// UpdateLastLogin updates the last login timestamp for a user
func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID int) error {
	query := `
		UPDATE users
		SET last_login = NOW()
		WHERE id = $1
	`
	
	_, err := r.db.ExecContext(ctx, query, userID)
	return err
}

// Update updates a user's information
func (r *UserRepository) Update(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET username = $1, email = $2, role = $3, is_active = $4, email_verified = $5
		WHERE id = $6
	`
	
	result, err := r.db.ExecContext(
		ctx,
		query,
		user.Username,
		user.Email,
		user.Role,
		user.IsActive,
		user.EmailVerified,
		user.ID,
	)
	
	if err != nil {
		if isUniqueViolation(err) {
			return ErrUserExists
		}
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return ErrUserNotFound
	}
	
	return nil
}

// Delete deletes a user from the database
func (r *UserRepository) Delete(ctx context.Context, userID int) error {
	query := `DELETE FROM users WHERE id = $1`
	
	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return err
	}
	
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	
	if rows == 0 {
		return ErrUserNotFound
	}
	
	return nil
}

// List retrieves all users with optional filtering
func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]*User, error) {
	query := `
		SELECT id, username, email, password_hash, role, is_active, email_verified,
		       created_at, updated_at, last_login
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`
	
	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var users []*User
	for rows.Next() {
		user := &User{}
		err := rows.Scan(
			&user.ID,
			&user.Username,
			&user.Email,
			&user.PasswordHash,
			&user.Role,
			&user.IsActive,
			&user.EmailVerified,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.LastLogin,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	
	if err = rows.Err(); err != nil {
		return nil, err
	}
	
	return users, nil
}

// isUniqueViolation checks if an error is a unique constraint violation
// This is PostgreSQL-specific but can be adapted for other databases
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// Check for PostgreSQL unique violation error code (23505)
	return err.Error() == "pq: duplicate key value violates unique constraint \"users_username_key\"" ||
		err.Error() == "pq: duplicate key value violates unique constraint \"users_email_key\""
}
