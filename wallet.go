package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrWalletNotFound   = errors.New("wallet not found")
	ErrInsufficientFund = errors.New("insufficient wallet balance")
	ErrDuplicateTx      = errors.New("ledger transaction reference already processed")
)

type Wallet struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	BalanceKobo int64     `json:"balance_kobo"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LedgerEntry struct {
	ID               int64     `json:"id"`
	WalletID         int64     `json:"wallet_id"`
	AmountKobo       int64     `json:"amount_kobo"`
	BalanceAfterKobo int64     `json:"balance_after_kobo"`
	EntryType        string    `json:"entry_type"`
	Reference        string    `json:"reference"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
}

// GetWalletByUserID retrieves the user's wallet, creating one if it doesn't exist
func GetWalletByUserID(ctx context.Context, db *sql.DB, userID int64) (*Wallet, error) {
	query := `
		INSERT INTO wallets (user_id, balance_kobo)
		VALUES ($1, 0)
		ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
		RETURNING id, user_id, balance_kobo, created_at, updated_at;
	`
	w := &Wallet{}
	err := db.QueryRowContext(ctx, query, userID).Scan(
		&w.ID,
		&w.UserID,
		&w.BalanceKobo,
		&w.CreatedAt,
		&w.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create wallet: %w", err)
	}
	return w, nil
}

// CreditWallet safely locks the wallet row, credits amountKobo, and writes a ledger entry
func CreditWallet(ctx context.Context, tx *sql.Tx, userID int64, amountKobo int64, reference, description string) (*LedgerEntry, error) {
	if amountKobo <= 0 {
		return nil, errors.New("credit amount must be positive")
	}

	// 1. Ensure wallet exists and lock row for update
	initQuery := `
		INSERT INTO wallets (user_id, balance_kobo)
		VALUES ($1, 0)
		ON CONFLICT (user_id) DO NOTHING;
	`
	if _, err := tx.ExecContext(ctx, initQuery, userID); err != nil {
		return nil, fmt.Errorf("failed to ensure wallet exists: %w", err)
	}

	var walletID int64
	var currentBalance int64
	lockQuery := `SELECT id, balance_kobo FROM wallets WHERE user_id = $1 FOR UPDATE`
	err := tx.QueryRowContext(ctx, lockQuery, userID).Scan(&walletID, &currentBalance)
	if err != nil {
		return nil, fmt.Errorf("error locking wallet: %w", err)
	}

	newBalance := currentBalance + amountKobo

	// 2. Update wallet balance
	updateQuery := `UPDATE wallets SET balance_kobo = $1, updated_at = NOW() WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateQuery, newBalance, walletID); err != nil {
		return nil, fmt.Errorf("failed to update wallet balance: %w", err)
	}

	// 3. Write immutable ledger entry
	ledgerQuery := `
		INSERT INTO ledger_entries (wallet_id, amount_kobo, balance_after_kobo, entry_type, reference, description)
		VALUES ($1, $2, $3, 'WALLET_FUNDING', $4, $5)
		RETURNING id, wallet_id, amount_kobo, balance_after_kobo, entry_type, reference, description, created_at;
	`
	entry := &LedgerEntry{}
	err = tx.QueryRowContext(ctx, ledgerQuery, walletID, amountKobo, newBalance, reference, description).Scan(
		&entry.ID,
		&entry.WalletID,
		&entry.AmountKobo,
		&entry.BalanceAfterKobo,
		&entry.EntryType,
		&entry.Reference,
		&entry.Description,
		&entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credit ledger entry: %w", err)
	}

	return entry, nil
}

// DebitWallet locks wallet row, checks sufficient funds, debits amountKobo, and writes ledger entry
func DebitWallet(ctx context.Context, tx *sql.Tx, userID int64, amountKobo int64, reference, description, entryType string) (*LedgerEntry, error) {
	if amountKobo <= 0 {
		return nil, errors.New("debit amount must be positive")
	}

	// 1. Lock wallet row for update
	var walletID int64
	var currentBalance int64
	lockQuery := `SELECT id, balance_kobo FROM wallets WHERE user_id = $1 FOR UPDATE`
	err := tx.QueryRowContext(ctx, lockQuery, userID).Scan(&walletID, &currentBalance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("error locking wallet: %w", err)
	}

	if currentBalance < amountKobo {
		return nil, ErrInsufficientFund
	}

	newBalance := currentBalance - amountKobo

	// 2. Update wallet balance
	updateQuery := `UPDATE wallets SET balance_kobo = $1, updated_at = NOW() WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateQuery, newBalance, walletID); err != nil {
		return nil, fmt.Errorf("failed to deduct wallet balance: %w", err)
	}

	// 3. Write immutable ledger entry
	ledgerQuery := `
		INSERT INTO ledger_entries (wallet_id, amount_kobo, balance_after_kobo, entry_type, reference, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, wallet_id, amount_kobo, balance_after_kobo, entry_type, reference, description, created_at;
	`
	entry := &LedgerEntry{}
	err = tx.QueryRowContext(ctx, ledgerQuery, walletID, -amountKobo, newBalance, entryType, reference, description).Scan(
		&entry.ID,
		&entry.WalletID,
		&entry.AmountKobo,
		&entry.BalanceAfterKobo,
		&entry.EntryType,
		&entry.Reference,
		&entry.Description,
		&entry.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert debit ledger entry: %w", err)
	}

	return entry, nil
}

// GetLedgerEntriesByWalletID fetches the recent ledger entries for a given wallet
func GetLedgerEntriesByWalletID(ctx context.Context, db *sql.DB, walletID int64, limit int) ([]LedgerEntry, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	query := `
		SELECT id, wallet_id, amount_kobo, balance_after_kobo, entry_type, reference, description, created_at
		FROM ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2;
	`
	rows, err := db.QueryContext(ctx, query, walletID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []LedgerEntry
	for rows.Next() {
		var e LedgerEntry
		if err := rows.Scan(
			&e.ID,
			&e.WalletID,
			&e.AmountKobo,
			&e.BalanceAfterKobo,
			&e.EntryType,
			&e.Reference,
			&e.Description,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan ledger entry: %w", err)
		}
		entries = append(entries, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating ledger rows: %w", err)
	}

	return entries, nil
}
