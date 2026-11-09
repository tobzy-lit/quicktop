package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestWalletOperations_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Create a dummy test user
	uniqueEmail := fmt.Sprintf("wallet_test_%d@example.com", time.Now().UnixNano())
	hash, err := hashPassword("TestPassword123!")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := &User{
		Name:         "Wallet User",
		Email:        uniqueEmail,
		PasswordHash: hash,
	}

	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// 2. Fetch or create user wallet
	wallet, err := GetWalletByUserID(ctx, db, user.ID)
	if err != nil {
		t.Fatalf("failed to get wallet: %v", err)
	}
	if wallet.BalanceKobo != 0 {
		t.Errorf("expected initial balance 0, got %d", wallet.BalanceKobo)
	}

	// 3. Credit wallet (₦500 -> 50,000 Kobo)
	creditRef := fmt.Sprintf("FUND-%d", time.Now().UnixNano())
	txCredit, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin credit tx: %v", err)
	}

	entryCredit, err := CreditWallet(ctx, txCredit, user.ID, 50000, creditRef, "Test wallet funding")
	if err != nil {
		txCredit.Rollback()
		t.Fatalf("failed to credit wallet: %v", err)
	}
	if err := txCredit.Commit(); err != nil {
		t.Fatalf("failed to commit credit tx: %v", err)
	}

	if entryCredit.BalanceAfterKobo != 50000 {
		t.Errorf("expected balance after credit 50000, got %d", entryCredit.BalanceAfterKobo)
	}

	// 4. Debit wallet (₦200 -> 20,000 Kobo)
	debitRef := fmt.Sprintf("DEBIT-%d", time.Now().UnixNano())
	txDebit, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin debit tx: %v", err)
	}

	entryDebit, err := DebitWallet(ctx, txDebit, user.ID, 20000, debitRef, "Test airtime debit", "AIRTIME_PURCHASE")
	if err != nil {
		txDebit.Rollback()
		t.Fatalf("failed to debit wallet: %v", err)
	}
	if err := txDebit.Commit(); err != nil {
		t.Fatalf("failed to commit debit tx: %v", err)
	}

	if entryDebit.BalanceAfterKobo != 30000 {
		t.Errorf("expected balance after debit 30000, got %d", entryDebit.BalanceAfterKobo)
	}

	// 5. Test Insufficient Funds Protection (Attempting to debit ₦500 -> 50,000 Kobo with only 30,000 balance)
	overDebitRef := fmt.Sprintf("OVER-%d", time.Now().UnixNano())
	txOver, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin over-debit tx: %v", err)
	}
	defer txOver.Rollback()

	_, err = DebitWallet(ctx, txOver, user.ID, 50000, overDebitRef, "Overspend attempt", "AIRTIME_PURCHASE")
	if !errors.Is(err, ErrInsufficientFund) {
		t.Errorf("expected ErrInsufficientFund, got %v", err)
	}
}
