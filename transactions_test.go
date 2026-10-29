package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestTransactionRepository_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database for integration test: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testRef := fmt.Sprintf("TEST-AIR-%d", time.Now().UnixNano())

	tx := &Transaction{
		Reference:   testRef,
		Network:     "MTN",
		PhoneNumber: "08012345678",
		AmountKobo:  100000, // ₦1,000.00
		Status:      StatusPending,
	}

	// 1. Test Insert
	if err := createTransaction(ctx, db, tx); err != nil {
		t.Fatalf("createTransaction failed: %v", err)
	}

	if tx.ID == 0 {
		t.Errorf("expected non-zero ID assigned by PostgreSQL, got %d", tx.ID)
	}

	// 2. Test Retrieve by Reference
	fetched, err := getTransactionByReference(ctx, db, testRef)
	if err != nil {
		t.Fatalf("getTransactionByReference failed: %v", err)
	}

	// 3. Verify retrieved fields match
	if fetched.Reference != testRef {
		t.Errorf("expected reference %s, got %s", testRef, fetched.Reference)
	}
	if fetched.AmountKobo != 100000 {
		t.Errorf("expected amount_kobo 100000, got %d", fetched.AmountKobo)
	}
	if fetched.Status != StatusPending {
		t.Errorf("expected status %s, got %s", StatusPending, fetched.Status)
	}
	if fetched.Network != "MTN" {
		t.Errorf("expected network MTN, got %s", fetched.Network)
	}

	// 4. Test Non-existent Reference returns ErrTransactionNotFound
	_, err = getTransactionByReference(ctx, db, "NON_EXISTENT_REF_9999")
	if !errors.Is(err, ErrTransactionNotFound) {
		t.Errorf("expected ErrTransactionNotFound, got %v", err)
	}
}

func TestStateMachine_Transitions(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		{"PENDING to PROCESSING", StatusPending, StatusProcessing, true},
		{"PENDING to FAILED", StatusPending, StatusFailed, true},
		{"PENDING to SUCCESS (invalid bypass)", StatusPending, StatusSuccess, false},
		{"PROCESSING to SUCCESS", StatusProcessing, StatusSuccess, true},
		{"PROCESSING to FAILED", StatusProcessing, StatusFailed, true},
		{"PROCESSING to PENDING (invalid regression)", StatusProcessing, StatusPending, false},
		{"SUCCESS to FAILED (terminal)", StatusSuccess, StatusFailed, false},
		{"SUCCESS to PROCESSING (terminal)", StatusSuccess, StatusProcessing, false},
		{"FAILED to SUCCESS (terminal)", StatusFailed, StatusSuccess, false},
		{"Unknown status", "UNKNOWN", StatusSuccess, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidTransition(tc.from, tc.to)
			if got != tc.expected {
				t.Errorf("IsValidTransition(%q, %q) = %v; want %v", tc.from, tc.to, got, tc.expected)
			}
		})
	}
}

func TestUpdateTransactionStatus_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	testRef := fmt.Sprintf("TEST-STATE-%d", time.Now().UnixNano())
	tx := &Transaction{
		Reference:   testRef,
		Network:     "MTN",
		PhoneNumber: "08012345678",
		AmountKobo:  50000,
		Status:      StatusPending,
	}

	if err := createTransaction(ctx, db, tx); err != nil {
		t.Fatalf("createTransaction failed: %v", err)
	}

	// 1. Valid transition: PENDING -> PROCESSING
	err = updateTransactionStatus(ctx, db, testRef, StatusPending, StatusProcessing)
	if err != nil {
		t.Fatalf("expected valid transition PENDING->PROCESSING, got error: %v", err)
	}

	// 2. Reject stale transition (attempting PENDING->SUCCESS when already in PROCESSING)
	err = updateTransactionStatus(ctx, db, testRef, StatusPending, StatusSuccess)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}

	// 3. Valid transition: PROCESSING -> SUCCESS
	err = updateTransactionStatus(ctx, db, testRef, StatusProcessing, StatusSuccess)
	if err != nil {
		t.Fatalf("expected valid transition PROCESSING->SUCCESS, got error: %v", err)
	}

	// 4. Reject transition out of terminal state: SUCCESS -> FAILED
	err = updateTransactionStatus(ctx, db, testRef, StatusSuccess, StatusFailed)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition on terminal state, got %v", err)
	}
}
