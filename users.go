package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// User represents an account holder in QuickTop
type User struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"` // Never expose hash in JSON responses
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RegisterRequest represents the registration payload
type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterResponse represents the response on successful registration
type RegisterResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	User    User   `json:"user"`
}

// Validate checks registration fields and normalizes inputs
func (r *RegisterRequest) Validate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name is required")
	}

	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	if r.Email == "" {
		return errors.New("email is required")
	}

	// Validate standard email format using net/mail
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return errors.New("invalid email address format")
	}

	if len(r.Password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}

	if len(r.Password) > 72 {
		// Bcrypt truncates passwords over 72 bytes
		return errors.New("password cannot exceed 72 characters")
	}

	return nil
}

// hashPassword produces a secure bcrypt hash of a plaintext password
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// checkPasswordHash compares a plaintext password against a stored bcrypt hash
func checkPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

var ErrDuplicateEmail = errors.New("email address already registered")

// createUser inserts a new user record into PostgreSQL
func createUser(ctx context.Context, db *sql.DB, user *User) error {
	query := `
		INSERT INTO users (name, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at;
	`

	err := db.QueryRowContext(ctx, query, user.Name, user.Email, user.PasswordHash).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateEmail
		}
		return fmt.Errorf("failed to insert user: %w", err)
	}

	return nil
}
