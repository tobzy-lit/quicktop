package main

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrTopupNotFound         = errors.New("wallet topup record not found")
	ErrTopupAlreadyProcessed = errors.New("wallet topup already processed")
)

type WalletTopup struct {
	ID               int64     `json:"id"`
	UserID           int64     `json:"user_id"`
	Reference        string    `json:"reference"`
	GatewayReference string    `json:"gateway_reference,omitempty"`
	AmountKobo       int64     `json:"amount_kobo"`
	Gateway          string    `json:"gateway"`
	Status           string    `json:"status"` // PENDING, SUCCESS, FAILED
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// CreateTopup records an initiated topup attempt
func CreateTopup(ctx context.Context, db *sql.DB, topup *WalletTopup) error {
	query := `
		INSERT INTO wallet_topups (user_id, reference, amount_kobo, gateway, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'PENDING', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		RETURNING id, created_at, updated_at
	`
	if topup.Gateway == "" {
		topup.Gateway = "PAYSTACK"
	}
	topup.Status = "PENDING"

	return db.QueryRowContext(ctx, query, topup.UserID, topup.Reference, topup.AmountKobo, topup.Gateway).
		Scan(&topup.ID, &topup.CreatedAt, &topup.UpdatedAt)
}

// GetTopupByReferenceForUpdate acquires a row-level lock on the topup record
func GetTopupByReferenceForUpdate(ctx context.Context, tx *sql.Tx, reference string) (*WalletTopup, error) {
	query := `
		SELECT id, user_id, reference, COALESCE(gateway_reference, ''), amount_kobo, gateway, status, created_at, updated_at
		FROM wallet_topups
		WHERE reference = $1
		FOR UPDATE
	`
	var t WalletTopup
	err := tx.QueryRowContext(ctx, query, reference).Scan(
		&t.ID,
		&t.UserID,
		&t.Reference,
		&t.GatewayReference,
		&t.AmountKobo,
		&t.Gateway,
		&t.Status,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTopupNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateTopupStatus marks the topup with its terminal status inside an active transaction
func UpdateTopupStatus(ctx context.Context, tx *sql.Tx, reference, status, gatewayRef string) error {
	query := `
		UPDATE wallet_topups
		SET status = $1, gateway_reference = $2, updated_at = CURRENT_TIMESTAMP
		WHERE reference = $3
	`
	res, err := tx.ExecContext(ctx, query, status, gatewayRef, reference)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTopupNotFound
	}
	return nil
}
