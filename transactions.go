package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// generateTransactionReference produces a unique, collision-resistant identifier
// Format: QT-AIR-<UNIXNANO>-<HEX>
func generateTransactionReference() (string, error) {
	bytes := make([]byte, 8) // 8 random bytes = 16 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return fmt.Sprintf("QT-AIR-%d-%s", time.Now().UnixNano(), hex.EncodeToString(bytes)), nil
}

// Transaction status constants
const (
	StatusPending    = "PENDING"
	StatusProcessing = "PROCESSING"
	StatusSuccess    = "SUCCESS"
	StatusFailed     = "FAILED"
)

// Permitted state machine transitions
var validTransitions = map[string]map[string]bool{
	StatusPending: {
		StatusProcessing: true,
		StatusFailed:     true,
	},
	StatusProcessing: {
		StatusSuccess: true,
		StatusFailed:  true,
	},
	StatusSuccess: {},
	StatusFailed:  {},
}

// IsValidTransition verifies whether a transition from one status to another is legal
func IsValidTransition(from, to string) bool {
	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}
	return allowed[to]
}

// Transaction represents a row in the transactions table
type Transaction struct {
	ID          int64     `json:"id"`
	Reference   string    `json:"reference"`
	Network     string    `json:"network"`
	PhoneNumber string    `json:"phone_number"`
	AmountKobo  int64     `json:"amount_kobo"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Domain errors
var (
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrInvalidTransition   = errors.New("invalid status transition")
	ErrStatusConflict      = errors.New("transaction status was modified concurrently")
)

// createTransaction inserts a new pending airtime record into PostgreSQL
func createTransaction(ctx context.Context, db *sql.DB, t *Transaction) error {
	query := `
		INSERT INTO transactions (reference, network, phone_number, amount_kobo, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at;
	`

	if t.Status == "" {
		t.Status = StatusPending
	}

	err := db.QueryRowContext(ctx, query,
		t.Reference,
		t.Network,
		t.PhoneNumber,
		t.AmountKobo,
		t.Status,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to insert transaction: %w", err)
	}

	return nil
}

// getTransactionByReference retrieves an existing record by its unique reference
func getTransactionByReference(ctx context.Context, db *sql.DB, reference string) (*Transaction, error) {
	query := `
		SELECT id, reference, network, phone_number, amount_kobo, status, created_at, updated_at
		FROM transactions
		WHERE reference = $1;
	`

	var t Transaction
	err := db.QueryRowContext(ctx, query, reference).Scan(
		&t.ID,
		&t.Reference,
		&t.Network,
		&t.PhoneNumber,
		&t.AmountKobo,
		&t.Status,
		&t.CreatedAt,
		&t.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("failed to query transaction: %w", err)
	}

	return &t, nil
}

// updateTransactionStatus safely transitions a transaction from currentStatus to newStatus
func updateTransactionStatus(ctx context.Context, db *sql.DB, reference, currentStatus, newStatus string) error {
	if !IsValidTransition(currentStatus, newStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", ErrInvalidTransition, currentStatus, newStatus)
	}

	query := `
		UPDATE transactions
		SET status = $1, updated_at = NOW()
		WHERE reference = $2 AND status = $3;
	`

	result, err := db.ExecContext(ctx, query, newStatus, reference, currentStatus)
	if err != nil {
		return fmt.Errorf("failed to execute status update: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrStatusConflict
	}

	return nil
}
