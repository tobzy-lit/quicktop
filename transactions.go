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
	PhoneNumber string    `json:"phone_number"`
	Network     string    `json:"network"`
	AmountKobo  int64     `json:"amount_kobo"`
	Status      string    `json:"status"`
	UserID      *int64    `json:"user_id,omitempty"`
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
func createTransaction(ctx context.Context, db *sql.DB, tx *Transaction) error {
	query := `
		INSERT INTO transactions (reference, phone_number, network, amount_kobo, status, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at;
	`

	err := db.QueryRowContext(
		ctx,
		query,
		tx.Reference,
		tx.PhoneNumber,
		tx.Network,
		tx.AmountKobo,
		tx.Status,
		tx.UserID,
	).Scan(&tx.ID, &tx.CreatedAt, &tx.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert transaction: %w", err)
	}

	return nil
}

// getTransactionByReference retrieves an existing record by its unique reference

func getTransactionByReference(ctx context.Context, db *sql.DB, ref string) (*Transaction, error) {
	query := `
		SELECT id, reference, phone_number, network, amount_kobo, status, user_id, created_at, updated_at
		FROM transactions
		WHERE reference = $1;
	`

	var tx Transaction
	err := db.QueryRowContext(ctx, query, ref).Scan(
		&tx.ID,
		&tx.Reference,
		&tx.PhoneNumber,
		&tx.Network,
		&tx.AmountKobo,
		&tx.Status,
		&tx.UserID,
		&tx.CreatedAt,
		&tx.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, err
	}

	return &tx, nil
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

// getTransactionsByUserID retrieves all transactions linked to a specific user ordered chronologically
func getTransactionsByUserID(ctx context.Context, db *sql.DB, userID int64) ([]Transaction, error) {
	query := `
		SELECT id, reference, phone_number, network, amount_kobo, status, user_id, created_at, updated_at
		FROM transactions
		WHERE user_id = $1
		ORDER BY created_at DESC;
	`

	rows, err := db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user transactions: %w", err)
	}
	defer rows.Close()

	// Initialize as an empty slice so it serializes to JSON [] rather than null
	transactions := make([]Transaction, 0)
	for rows.Next() {
		var tx Transaction
		err := rows.Scan(
			&tx.ID,
			&tx.Reference,
			&tx.PhoneNumber,
			&tx.Network,
			&tx.AmountKobo,
			&tx.Status,
			&tx.UserID,
			&tx.CreatedAt,
			&tx.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user transaction row: %w", err)
		}
		transactions = append(transactions, tx)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return transactions, nil
}

// createTransactionTx inserts a new transaction record within an existing SQL transaction
func createTransactionTx(ctx context.Context, tx *sql.Tx, tRecord *Transaction) error {
	query := `
		INSERT INTO transactions (reference, phone_number, network, amount_kobo, status, user_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at;
	`
	err := tx.QueryRowContext(
		ctx,
		query,
		tRecord.Reference,
		tRecord.PhoneNumber,
		tRecord.Network,
		tRecord.AmountKobo,
		tRecord.Status,
		tRecord.UserID,
	).Scan(&tRecord.ID, &tRecord.CreatedAt, &tRecord.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert transaction in tx: %w", err)
	}

	return nil
}
