package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	// 1. Fetch configuration from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "dev-secret-quicktop-insecure-local-only"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	// 2. Initialize PostgreSQL connection pool
	db, err := initDB(dbURL)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer db.Close()

	// 3. Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler(db))
	mux.HandleFunc("/api/v1/networks", networksHandler)
	mux.HandleFunc("/api/v1/airtime/validate", validateAirtimeHandler)
	mux.HandleFunc("/api/v1/airtime/purchase", purchaseAirtimeHandler(db))
	mux.HandleFunc("/api/v1/transactions/", getTransactionHandler(db))
	mux.HandleFunc("/api/v1/auth/register", registerHandler(db))
	mux.HandleFunc("/api/v1/auth/login", loginHandler(db, jwtSecret))

	// 4. Configure HTTP server with safety timeouts
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("QuickTop server starting on port %s...", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server stopped unexpectedly: %v", err)
	}
}
