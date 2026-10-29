package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPurchaseAndRetrieveAirtime_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 1. Test POST /api/v1/airtime/purchase - Success creation
	payload := `{"network": "GLO", "phone_number": "08051234567", "amount": 200}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := purchaseAirtimeHandler(db)
	handler.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d", resp.StatusCode)
	}

	var purchaseRes AirtimePurchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&purchaseRes); err != nil {
		t.Fatalf("failed to decode purchase response: %v", err)
	}

	if purchaseRes.Reference == "" {
		t.Errorf("expected non-empty transaction reference")
	}
	if purchaseRes.TxStatus != StatusPending {
		t.Errorf("expected initial status %s, got %s", StatusPending, purchaseRes.TxStatus)
	}
	if purchaseRes.AmountKobo != 20000 {
		t.Errorf("expected 20000 kobo, got %d", purchaseRes.AmountKobo)
	}

	// 2. Test GET /api/v1/transactions/{reference} - Lookup
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/"+purchaseRes.Reference, nil)
	getW := httptest.NewRecorder()

	getHandler := getTransactionHandler(db)
	getHandler.ServeHTTP(getW, getReq)

	getResp := getW.Result()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK on lookup, got %d", getResp.StatusCode)
	}

	var detailRes TransactionDetailsResponse
	if err := json.NewDecoder(getResp.Body).Decode(&detailRes); err != nil {
		t.Fatalf("failed to decode retrieval response: %v", err)
	}

	if detailRes.Reference != purchaseRes.Reference {
		t.Errorf("expected reference %s, got %s", purchaseRes.Reference, detailRes.Reference)
	}

	// 3. Test GET /api/v1/transactions/{reference} - 404 Not Found
	notReq := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/QT-NON-EXISTENT-XYZ", nil)
	notW := httptest.NewRecorder()

	getHandler.ServeHTTP(notW, notReq)
	if notW.Result().StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 Not Found, got %d", notW.Result().StatusCode)
	}
}
func TestRegisterHandler_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	handler := registerHandler(db)
	uniqueEmail := fmt.Sprintf("handler_test_%d@example.com", time.Now().UnixNano())

	// 1. Success case: 201 Created
	validPayload := fmt.Sprintf(`{"name": "Handler Test User", "email": "%s", "password": "StrongPassword123!"}`, uniqueEmail)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(validPayload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d", resp.StatusCode)
	}

	var regRes RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&regRes); err != nil {
		t.Fatalf("failed to decode register response: %v", err)
	}
	if regRes.User.ID <= 0 {
		t.Errorf("expected valid user ID, got %d", regRes.User.ID)
	}
	if regRes.User.Email != uniqueEmail {
		t.Errorf("expected email %s, got %s", uniqueEmail, regRes.User.Email)
	}
	if regRes.User.PasswordHash != "" {
		t.Errorf("password hash must not be exposed in response")
	}

	// 2. Duplicate email: 409 Conflict
	dupReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(validPayload))
	dupReq.Header.Set("Content-Type", "application/json")
	dupW := httptest.NewRecorder()

	handler.ServeHTTP(dupW, dupReq)
	if dupW.Result().StatusCode != http.StatusConflict {
		t.Errorf("expected status 409 Conflict for duplicate email, got %d", dupW.Result().StatusCode)
	}

	// 3. Invalid input (short password): 400 Bad Request
	badPayload := `{"name": "Bad Pass", "email": "badpass@example.com", "password": "123"}`
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(badPayload))
	badReq.Header.Set("Content-Type", "application/json")
	badW := httptest.NewRecorder()

	handler.ServeHTTP(badW, badReq)
	if badW.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", badW.Result().StatusCode)
	}
}
