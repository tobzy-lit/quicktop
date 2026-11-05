package main

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

// getUserIDFromContext retrieves the user ID stored in request context, if any
func getUserIDFromContext(ctx context.Context) (int64, bool) {
	val, ok := ctx.Value(userIDContextKey).(int64)
	return val, ok
}

// requireAuthMiddleware enforces a valid JWT in the Authorization header
func requireAuthMiddleware(jwtSecret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Missing authorization header",
			})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Invalid authorization format, expected 'Bearer <token>'",
			})
			return
		}

		tokenString := parts[1]
		claims, err := verifyToken(tokenString, jwtSecret)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Invalid or expired token",
			})
			return
		}

		// Attach user ID to the request context
		ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// optionalAuthMiddleware extracts user ID if a valid token is provided,
// but allows unauthenticated guest requests through with no user ID in context.
func optionalAuthMiddleware(jwtSecret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			claims, err := verifyToken(parts[1], jwtSecret)
			if err == nil {
				ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
				r = r.WithContext(ctx)
			}
		}

		next.ServeHTTP(w, r)
	}
}

// corsMiddleware enables Cross-Origin Resource Sharing for frontend clients
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
