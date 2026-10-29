package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
