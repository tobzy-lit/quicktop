package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

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

// ErrTransactionNotFound is returned when a requested reference doesn't exist
var ErrTransactionNotFound = errors.New("transaction not found")

// createTransaction inserts a new pending airtime record into PostgreSQL using parameters ($1, $2, etc.)
func createTransaction(ctx context.Context, db *sql.DB, t *Transaction) error {
	query := `
		INSERT INTO transactions (reference, network, phone_number, amount_kobo, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at;
	`

	// Default status to PENDING if not provided
	if t.Status == "" {
		t.Status = "PENDING"
	}

	// QueryRowContext executes the parameterized query and reads back the generated ID and timestamps
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
