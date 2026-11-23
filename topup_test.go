package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func signPaystackPayload(payload []byte, secret string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestTopupAndPaystackWebhook_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Skipf("Skipping integration test, DB unavailable: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	jwtSecret := "test-jwt-secret-milestone7"
	paystackSecret := "sk_test_milestone7_secret_key"

	// Mock Paystack API server
	var lastGeneratedRef string
	mockPaystackServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/transaction/initialize":
			var req paystackInitRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			lastGeneratedRef = req.Reference

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(paystackInitResponse{
				Status:  true,
				Message: "Authorization URL created",
				Data: PaystackInitData{
					AuthorizationURL: "https://checkout.paystack.com/mock_" + req.Reference,
					AccessCode:       "ACCESS_MOCK_123",
					Reference:        req.Reference,
				},
			})
		case "/transaction/verify/" + lastGeneratedRef:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(paystackVerifyResponse{
				Status:  true,
				Message: "Verification successful",
				Data: PaystackVerifyData{
					Status:          "success",
					Reference:       lastGeneratedRef,
					Amount:          250000, // ₦2,500.00
					GatewayResponse: "Successful",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockPaystackServer.Close()

	paystackClient := &PaystackClient{
		SecretKey:  paystackSecret,
		BaseURL:    mockPaystackServer.URL,
		HTTPClient: mockPaystackServer.Client(),
	}

	// Create isolated test user
	email := fmt.Sprintf("topup_tester_%d@quicktop.ng", time.Now().UnixNano())
	hash, err := hashPassword("StrongPass123!")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	user := &User{Email: email, PasswordHash: hash}
	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	token, err := generateToken(user.ID, user.Email, jwtSecret, time.Hour)
	if err != nil {
		t.Fatalf("Failed to generate JWT: %v", err)
	}

	initHandler := requireAuthMiddleware(jwtSecret, initializeTopupHandler(db, paystackClient))
	webhookHandler := paystackWebhookHandler(db, paystackSecret)
	verifyHandler := requireAuthMiddleware(jwtSecret, verifyTopupHandler(db, paystackClient))

	t.Run("Rejects invalid topup amount below minimum", func(t *testing.T) {
		body := bytes.NewBufferString(`{"amount_naira": 50}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/fund/initialize", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		initHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("Expected 400 Bad Request, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	var topupRef string
	t.Run("Initializes topup and creates PENDING record", func(t *testing.T) {
		body := bytes.NewBufferString(`{"amount_naira": 2500}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/fund/initialize", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		initHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to parse init response: %v", err)
		}
		topupRef, _ = resp["reference"].(string)
		if topupRef == "" {
			t.Fatalf("Expected non-empty topup reference")
		}
	})

	t.Run("Rejects webhook with invalid HMAC signature", func(t *testing.T) {
		payload := []byte(fmt.Sprintf(`{"event":"charge.success","data":{"reference":"%s","amount":250000,"status":"success","id":999111}}`, topupRef))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paystack", bytes.NewReader(payload))
		req.Header.Set("X-Paystack-Signature", "forged_invalid_signature_value")
		rec := httptest.NewRecorder()

		webhookHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("Expected 401 Unauthorized for invalid signature, got %d", rec.Code)
		}
	})

	t.Run("Credits wallet atomically on valid webhook and enforces idempotency on replay", func(t *testing.T) {
		payload := []byte(fmt.Sprintf(`{"event":"charge.success","data":{"reference":"%s","amount":250000,"status":"success","id":999222}}`, topupRef))
		validSig := signPaystackPayload(payload, paystackSecret)

		// First delivery
		req1 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paystack", bytes.NewReader(payload))
		req1.Header.Set("X-Paystack-Signature", validSig)
		rec1 := httptest.NewRecorder()
		webhookHandler.ServeHTTP(rec1, req1)

		if rec1.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK on valid webhook, got %d", rec1.Code)
		}

		wallet, err := GetWalletByUserID(ctx, db, user.ID)
		if err != nil {
			t.Fatalf("Failed to fetch wallet: %v", err)
		}
		if wallet.BalanceKobo != 250000 {
			t.Fatalf("Expected wallet balance 250000 kobo, got %d", wallet.BalanceKobo)
		}

		// Replay attack / duplicate webhook retry
		req2 := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paystack", bytes.NewReader(payload))
		req2.Header.Set("X-Paystack-Signature", validSig)
		rec2 := httptest.NewRecorder()
		webhookHandler.ServeHTTP(rec2, req2)

		if rec2.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK on duplicate webhook, got %d", rec2.Code)
		}

		walletAfterReplay, _ := GetWalletByUserID(ctx, db, user.ID)
		if walletAfterReplay.BalanceKobo != 250000 {
			t.Fatalf("Idempotency failed: expected 250000 kobo after replay, got %d", walletAfterReplay.BalanceKobo)
		}
	})

	t.Run("Manual verify endpoint succeeds idempotently", func(t *testing.T) {
		body := bytes.NewBufferString(fmt.Sprintf(`{"reference":"%s"}`, topupRef))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/wallet/fund/verify", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		verifyHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Expected 200 OK on verify, got %d: %s", rec.Code, rec.Body.String())
		}

		walletFinal, _ := GetWalletByUserID(ctx, db, user.ID)
		if walletFinal.BalanceKobo != 250000 {
			t.Fatalf("Expected balance to remain 250000 kobo, got %d", walletFinal.BalanceKobo)
		}
	})
}
