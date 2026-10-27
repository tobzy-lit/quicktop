package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestTransactionRepository_Integration(t *testing.T) {
	// 1. Check for database connection string
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	// 2. Connect to the test database
	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database for integration test: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 3. Create a unique reference for this test run
	testRef := fmt.Sprintf("TEST-AIR-%d", time.Now().UnixNano())

	tx := &Transaction{
		Reference:   testRef,
		Network:     "MTN",
		PhoneNumber: "08012345678",
		AmountKobo:  100000, // ₦1,000.00
		Status:      "PENDING",
	}

	// 4. Test Insert
	if err := createTransaction(ctx, db, tx); err != nil {
		t.Fatalf("createTransaction failed: %v", err)
	}

	if tx.ID == 0 {
		t.Errorf("expected non-zero ID assigned by PostgreSQL, got %d", tx.ID)
	}

	// 5. Test Retrieve by Reference
	fetched, err := getTransactionByReference(ctx, db, testRef)
	if err != nil {
		t.Fatalf("getTransactionByReference failed: %v", err)
	}

	// 6. Verify retrieved fields match
	if fetched.Reference != testRef {
		t.Errorf("expected reference %s, got %s", testRef, fetched.Reference)
	}
	if fetched.AmountKobo != 100000 {
		t.Errorf("expected amount_kobo 100000, got %d", fetched.AmountKobo)
	}
	if fetched.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %s", fetched.Status)
	}
	if fetched.Network != "MTN" {
		t.Errorf("expected network MTN, got %s", fetched.Network)
	}

	// 7. Test Non-existent Reference returns ErrTransactionNotFound
	_, err = getTransactionByReference(ctx, db, "NON_EXISTENT_REF_9999")
	if err != ErrTransactionNotFound {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}
