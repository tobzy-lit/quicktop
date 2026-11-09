package main

import (
	"context"
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

	handler := purchaseAirtimeHandler(db, NewSimulatorProvider())
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

func TestLoginHandler_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	jwtSecret := "test-suite-jwt-secret-key"
	uniqueEmail := fmt.Sprintf("login_test_%d@example.com", time.Now().UnixNano())
	rawPassword := "ValidUserPassword123!"

	// Seed user account for testing login
	hash, err := hashPassword(rawPassword)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := &User{
		Name:         "Login Test Subject",
		Email:        uniqueEmail,
		PasswordHash: hash,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("failed to seed test user: %v", err)
	}

	handler := loginHandler(db, jwtSecret)

	// 1. Valid login credentials -> 200 OK with valid JWT
	validBody := fmt.Sprintf(`{"email": "%s", "password": "%s"}`, uniqueEmail, rawPassword)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	var loginRes LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginRes); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}

	if loginRes.Token == "" {
		t.Fatalf("expected non-empty auth token in login response")
	}

	// Verify the issued token can be parsed and matches claims
	claims, err := verifyToken(loginRes.Token, jwtSecret)
	if err != nil {
		t.Fatalf("issued token failed verification: %v", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("expected UserID %d, got %d", user.ID, claims.UserID)
	}
	if claims.Email != uniqueEmail {
		t.Errorf("expected email %s, got %s", uniqueEmail, claims.Email)
	}

	// 2. Incorrect password -> 401 Unauthorized
	badPassBody := fmt.Sprintf(`{"email": "%s", "password": "WrongPasswordXYZ"}`, uniqueEmail)
	badPassReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(badPassBody))
	badPassReq.Header.Set("Content-Type", "application/json")
	badPassW := httptest.NewRecorder()

	handler.ServeHTTP(badPassW, badPassReq)
	if badPassW.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for incorrect password, got %d", badPassW.Result().StatusCode)
	}

	// 3. Unknown email -> 401 Unauthorized
	unknownBody := `{"email": "nobody_exists_here@example.com", "password": "AnyPassword123"}`
	unknownReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(unknownBody))
	unknownReq.Header.Set("Content-Type", "application/json")
	unknownW := httptest.NewRecorder()

	handler.ServeHTTP(unknownW, unknownReq)
	if unknownW.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401 for unknown user, got %d", unknownW.Result().StatusCode)
	}

	// 4. Missing required field -> 400 Bad Request
	emptyBody := fmt.Sprintf(`{"email": "%s", "password": ""}`, uniqueEmail)
	emptyReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(emptyBody))
	emptyReq.Header.Set("Content-Type", "application/json")
	emptyW := httptest.NewRecorder()

	handler.ServeHTTP(emptyW, emptyReq)
	if emptyW.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty password, got %d", emptyW.Result().StatusCode)
	}
}

func TestAuthenticatedPurchase_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	jwtSecret := "integration-test-secret"
	email := fmt.Sprintf("auth_tx_%d@example.com", time.Now().UnixNano())

	// 1. Seed a user
	user := &User{
		Name:         "Auth Tx User",
		Email:        email,
		PasswordHash: "dummyhash",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("failed to seed test user: %v", err)
	}

	// 2. Generate a token for this user
	token, err := generateToken(user.ID, user.Email, jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// 3. Wrap purchase handler with optionalAuthMiddleware
	handler := optionalAuthMiddleware(jwtSecret, purchaseAirtimeHandler(db, NewSimulatorProvider()))

	payload := `{"network": "GLO", "phone_number": "08051234567", "amount": 200}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d", resp.StatusCode)
	}

	var purchaseRes AirtimePurchaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&purchaseRes); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if purchaseRes.Reference == "" {
		t.Fatalf("expected non-empty reference in purchase response")
	}

	// 4. Retrieve from DB and assert user_id matches
	tx, err := getTransactionByReference(ctx, db, purchaseRes.Reference)
	if err != nil {
		t.Fatalf("failed to look up transaction: %v", err)
	}

	if tx.UserID == nil {
		t.Fatalf("expected transaction user_id to be set, got nil")
	}

	if *tx.UserID != user.ID {
		t.Errorf("expected transaction user_id %d, got %d", user.ID, *tx.UserID)
	}
}

func TestUserTransactions_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	jwtSecret := "history-test-secret"
	email := fmt.Sprintf("history_user_%d@example.com", time.Now().UnixNano())

	// 1. Seed user
	user := &User{
		Name:         "History Test User",
		Email:        email,
		PasswordHash: "dummyhash",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("failed to seed test user: %v", err)
	}

	token, err := generateToken(user.ID, user.Email, jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	handler := requireAuthMiddleware(jwtSecret, userTransactionsHandler(db))

	// 2. Unauthenticated request -> 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/api/v1/user/transactions", nil)
	unauthW := httptest.NewRecorder()
	handler.ServeHTTP(unauthW, unauthReq)
	if unauthW.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated request, got %d", unauthW.Result().StatusCode)
	}

	// 3. Seed two transactions for this user
	tx1 := &Transaction{
		Reference:   fmt.Sprintf("QT-TEST-HIST-1-%d", time.Now().UnixNano()),
		PhoneNumber: "08030000001",
		Network:     "MTN",
		AmountKobo:  10000,
		Status:      StatusPending,
		UserID:      &user.ID,
	}
	if err := createTransaction(ctx, db, tx1); err != nil {
		t.Fatalf("failed to insert tx1: %v", err)
	}

	tx2 := &Transaction{
		Reference:   fmt.Sprintf("QT-TEST-HIST-2-%d", time.Now().UnixNano()),
		PhoneNumber: "08030000002",
		Network:     "GLO",
		AmountKobo:  20000,
		Status:      StatusSuccess,
		UserID:      &user.ID,
	}
	if err := createTransaction(ctx, db, tx2); err != nil {
		t.Fatalf("failed to insert tx2: %v", err)
	}

	// Seed one guest transaction (should NOT be returned)
	guestTx := &Transaction{
		Reference:   fmt.Sprintf("QT-TEST-GUEST-%d", time.Now().UnixNano()),
		PhoneNumber: "08039999999",
		Network:     "AIRTEL",
		AmountKobo:  50000,
		Status:      StatusPending,
		UserID:      nil,
	}
	if err := createTransaction(ctx, db, guestTx); err != nil {
		t.Fatalf("failed to insert guestTx: %v", err)
	}

	// 4. Authenticated request -> 200 OK with isolated results
	authReq := httptest.NewRequest(http.MethodGet, "/api/v1/user/transactions", nil)
	authReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	authW := httptest.NewRecorder()
	handler.ServeHTTP(authW, authReq)

	resp := authW.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	var res struct {
		Status string        `json:"status"`
		Count  int           `json:"count"`
		Data   []Transaction `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if res.Count < 2 {
		t.Errorf("expected count >= 2, got %d", res.Count)
	}

	// Assert all returned items belong exclusively to this user
	for _, item := range res.Data {
		if item.UserID == nil || *item.UserID != user.ID {
			t.Errorf("data leak: returned transaction %s has user_id %v, expected %d", item.Reference, item.UserID, user.ID)
		}
	}
}
