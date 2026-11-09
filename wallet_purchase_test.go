package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestAtomicWalletAirtimePurchase_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("failed to connect to test db: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jwtSecret := "test_secret_for_wallet_spend_tests"
	provider := NewSimulatorProvider()

	// Helper to create test user with pre-funded wallet
	setupUserWithBalance := func(email string, initialKobo int64) (*User, string) {
		hash, _ := hashPassword("Secret123!")
		user := &User{Name: "Wallet Spender", Email: email, PasswordHash: hash}
		if err := createUser(ctx, db, user); err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		if initialKobo > 0 {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatalf("failed to begin tx: %v", err)
			}
			ref := fmt.Sprintf("INIT-FUND-%d", time.Now().UnixNano())
			if _, err := CreditWallet(ctx, tx, user.ID, initialKobo, ref, "Initial funding"); err != nil {
				tx.Rollback()
				t.Fatalf("failed to credit initial fund: %v", err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatalf("failed to commit fund tx: %v", err)
			}
		}

		token, err := generateToken(user.ID, user.Email, jwtSecret, 1*time.Hour)
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}
		return user, token
	}

	handler := optionalAuthMiddleware(jwtSecret, purchaseAirtimeHandler(db, provider))

	// -------------------------------------------------------------------------
	// Scenario 1: Successful purchase with atomic wallet debit
	// -------------------------------------------------------------------------
	t.Run("Successful airtime purchase debits wallet atomically", func(t *testing.T) {
		user, token := setupUserWithBalance(fmt.Sprintf("user_spend_ok_%d@test.com", time.Now().UnixNano()), 100000) // ₦1,000

		reqBody := `{"phone_number": "08031234567", "network": "MTN", "amount": 200}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
		}

		// Verify wallet balance is ₦800 (80,000 Kobo)
		wallet, err := GetWalletByUserID(ctx, db, user.ID)
		if err != nil {
			t.Fatalf("failed to get wallet: %v", err)
		}
		if wallet.BalanceKobo != 80000 {
			t.Errorf("expected balance 80000 kobo, got %d", wallet.BalanceKobo)
		}

		// Verify ledger entry
		entries, err := GetLedgerEntriesByWalletID(ctx, db, wallet.ID, 5)
		if err != nil {
			t.Fatalf("failed to get ledger entries: %v", err)
		}
		if len(entries) < 2 {
			t.Fatalf("expected at least 2 ledger entries (fund + debit), got %d", len(entries))
		}
		if entries[0].EntryType != "AIRTIME_PURCHASE" {
			t.Errorf("expected latest entry type AIRTIME_PURCHASE, got %s", entries[0].EntryType)
		}
		if entries[0].AmountKobo != -20000 {
			t.Errorf("expected debit amount -20000 kobo, got %d", entries[0].AmountKobo)
		}
	})

	// -------------------------------------------------------------------------
	// Scenario 2: Insufficient balance returns 422 and prevents deduction
	// -------------------------------------------------------------------------
	t.Run("Insufficient balance returns 422 and aborts transaction", func(t *testing.T) {
		user, token := setupUserWithBalance(fmt.Sprintf("user_low_bal_%d@test.com", time.Now().UnixNano()), 10000) // ₦100

		// Try to purchase ₦500
		reqBody := `{"phone_number": "08031234567", "network": "MTN", "amount": 500}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422 Unprocessable Entity, got %d: %s", rec.Code, rec.Body.String())
		}

		// Ensure balance remained untouched at 10,000 Kobo
		wallet, err := GetWalletByUserID(ctx, db, user.ID)
		if err != nil {
			t.Fatalf("failed to get wallet: %v", err)
		}
		if wallet.BalanceKobo != 10000 {
			t.Errorf("expected balance unchanged at 10000, got %d", wallet.BalanceKobo)
		}
	})

	// -------------------------------------------------------------------------
	// Scenario 3: Failed purchase automatically refunds deducted balance
	// -------------------------------------------------------------------------
	t.Run("Failed airtime delivery triggers automated wallet refund", func(t *testing.T) {
		user, token := setupUserWithBalance(fmt.Sprintf("user_refund_%d@test.com", time.Now().UnixNano()), 50000) // ₦500

		// Numbers ending in 0000 trigger simulated delivery failure
		reqBody := `{"phone_number": "08030000000", "network": "MTN", "amount": 300}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created on initiation, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp AirtimePurchaseResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// Allow background goroutine to process and refund
		time.Sleep(200 * time.Millisecond)

		// Verify transaction failed
		txRecord, err := getTransactionByReference(ctx, db, resp.Reference)
		if err != nil {
			t.Fatalf("failed to query transaction: %v", err)
		}
		if txRecord.Status != StatusFailed {
			t.Errorf("expected transaction status FAILED, got %s", txRecord.Status)
		}

		// Verify wallet balance was restored to ₦500 (50,000 Kobo)
		wallet, err := GetWalletByUserID(ctx, db, user.ID)
		if err != nil {
			t.Fatalf("failed to get wallet: %v", err)
		}
		if wallet.BalanceKobo != 50000 {
			t.Errorf("expected balance restored to 50000 kobo after refund, got %d", wallet.BalanceKobo)
		}

		// Verify ledger contains the refund entry
		entries, err := GetLedgerEntriesByWalletID(ctx, db, wallet.ID, 5)
		if err != nil {
			t.Fatalf("failed to get ledger: %v", err)
		}
		if len(entries) < 3 {
			t.Fatalf("expected at least 3 entries (fund, debit, refund), got %d", len(entries))
		}
		if entries[0].AmountKobo != 30000 {
			t.Errorf("expected refund credit of +30000 kobo, got %d", entries[0].AmountKobo)
		}
	})
}
