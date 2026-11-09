package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestGetWalletHandler_Integration(t *testing.T) {
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
	uniqueEmail := fmt.Sprintf("wallet_api_%d@example.com", time.Now().UnixNano())
	hash, err := hashPassword("TestPassword123!")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := &User{
		Name:         "Wallet API User",
		Email:        uniqueEmail,
		PasswordHash: hash,
	}
	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	// 2. Fund the wallet with ₦1,500 (150,000 Kobo)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	ref := fmt.Sprintf("TEST-FUND-%d", time.Now().UnixNano())
	if _, err := CreditWallet(ctx, tx, user.ID, 150000, ref, "Initial test deposit"); err != nil {
		tx.Rollback()
		t.Fatalf("failed to credit wallet: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("failed to commit tx: %v", err)
	}

	// 3. Generate JWT with the full signature: (userID, email, secret, duration)
	jwtSecret := "test_jwt_secret_key_12345"
	token, err := generateToken(user.ID, user.Email, jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 4. Send GET /api/v1/wallet request through requireAuthMiddleware
	handler := requireAuthMiddleware(jwtSecret, getWalletHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp WalletDetailsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}

	if resp.BalanceKobo != 150000 {
		t.Errorf("expected balance_kobo 150000, got %d", resp.BalanceKobo)
	}
	if resp.BalanceNaira != 1500.0 {
		t.Errorf("expected balance_naira 1500.0, got %f", resp.BalanceNaira)
	}
	if len(resp.RecentLedger) != 1 {
		t.Fatalf("expected 1 ledger entry, got %d", len(resp.RecentLedger))
	}
	if resp.RecentLedger[0].Reference != ref {
		t.Errorf("expected reference %s, got %s", ref, resp.RecentLedger[0].Reference)
	}
}
