package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequireAuthMiddleware(t *testing.T) {
	jwtSecret := "middleware-test-secret"

	// Dummy handler that asserts user ID presence in context
	targetHandler := func(expectedID int64) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			userID, ok := getUserIDFromContext(r.Context())
			if !ok || userID != expectedID {
				t.Errorf("expected context user ID %d, got %d (found=%v)", expectedID, userID, ok)
			}
			w.WriteHeader(http.StatusOK)
		}
	}

	// 1. Missing Authorization header -> 401 Unauthorized
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	wrapped := requireAuthMiddleware(jwtSecret, targetHandler(1))
	wrapped.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", w.Result().StatusCode)
	}

	// 2. Malformed Authorization header -> 401 Unauthorized
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "InvalidHeaderFormat")
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for malformed header, got %d", w.Result().StatusCode)
	}

	// 3. Expired token -> 401 Unauthorized
	expiredToken, err := generateToken(99, "expired@example.com", jwtSecret, -5*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", expiredToken))
	w = httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", w.Result().StatusCode)
	}

	// 4. Valid token -> 200 OK and user ID available in context
	validToken, err := generateToken(101, "valid@example.com", jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate valid token: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", validToken))
	w = httptest.NewRecorder()
	wrapped = requireAuthMiddleware(jwtSecret, targetHandler(101))
	wrapped.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", w.Result().StatusCode)
	}
}

func TestOptionalAuthMiddleware(t *testing.T) {
	jwtSecret := "optional-middleware-secret"

	// 1. Guest request (no header) -> 200 OK, no user ID in context
	guestTarget := func(w http.ResponseWriter, r *http.Request) {
		_, ok := getUserIDFromContext(r.Context())
		if ok {
			t.Errorf("expected no user ID in context for guest request")
		}
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", nil)
	w := httptest.NewRecorder()
	wrappedGuest := optionalAuthMiddleware(jwtSecret, guestTarget)
	wrappedGuest.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 for guest request, got %d", w.Result().StatusCode)
	}

	// 2. Authenticated request -> 200 OK, user ID present in context
	authTarget := func(w http.ResponseWriter, r *http.Request) {
		userID, ok := getUserIDFromContext(r.Context())
		if !ok || userID != 202 {
			t.Errorf("expected context user ID 202, got %d (found=%v)", userID, ok)
		}
		w.WriteHeader(http.StatusOK)
	}

	validToken, err := generateToken(202, "logged_in@example.com", jwtSecret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/airtime/purchase", nil)
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", validToken))
	w = httptest.NewRecorder()
	wrappedAuth := optionalAuthMiddleware(jwtSecret, authTarget)
	wrappedAuth.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 for authenticated request, got %d", w.Result().StatusCode)
	}
}
