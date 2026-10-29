package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

// LoginRequest represents incoming credentials
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse returns the authentication token and user profile
type LoginResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Token   string `json:"token"`
	User    User   `json:"user"`
}

// AuthClaims holds the JWT claims payload
type AuthClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Validate validates login input
func (r *LoginRequest) Validate() error {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	return nil
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

// getUserByEmail retrieves a user by email address
func getUserByEmail(ctx context.Context, db *sql.DB, email string) (*User, error) {
	query := `
		SELECT id, name, email, password_hash, created_at, updated_at
		FROM users
		WHERE email = $1;
	`

	var user User
	err := db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// generateToken signs a new JWT token for an authenticated user
func generateToken(userID int64, email string, secret string, duration time.Duration) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret cannot be empty")
	}

	claims := AuthClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return signedToken, nil
}

// verifyToken validates a JWT string against the secret and returns its claims
func verifyToken(tokenString string, secret string) (*AuthClaims, error) {
	if secret == "" {
		return nil, errors.New("jwt secret cannot be empty")
	}

	token, err := jwt.ParseWithClaims(tokenString, &AuthClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Ensure the token's signing algorithm is HMAC
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*AuthClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return claims, nil
}
