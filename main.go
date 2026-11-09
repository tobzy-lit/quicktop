package main

import (
	"log"
	"net/http"
	"os"
	"strings"
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

	// 2. Initialize Airtime Provider (Simulator or VTpass)
	providerMode := strings.ToLower(os.Getenv("PROVIDER_MODE"))
	var airtimeProvider AirtimeProvider

	if providerMode == "vtpass" {
		baseURL := os.Getenv("VTPASS_BASE_URL")
		if baseURL == "" {
			baseURL = "https://sandbox.vtpass.com/api"
		}
		apiKey := os.Getenv("VTPASS_API_KEY")
		secretKey := os.Getenv("VTPASS_SECRET_KEY")
		publicKey := os.Getenv("VTPASS_PUBLIC_KEY")

		airtimeProvider = NewVTpassClient(baseURL, apiKey, secretKey, publicKey)
		log.Printf("[Provider] Initialized live provider: VTpass (Sandbox: %s)", baseURL)
	} else {
		airtimeProvider = NewSimulatorProvider()
		log.Printf("[Provider] Initialized local provider: Simulator")
	}

	// 3. Initialize PostgreSQL connection pool
	db, err := initDB(dbURL)
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer db.Close()

	// 4. Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler(db))
	mux.HandleFunc("/api/v1/networks", networksHandler)
	mux.HandleFunc("/api/v1/airtime/validate", validateAirtimeHandler)
	mux.HandleFunc("/api/v1/airtime/purchase", optionalAuthMiddleware(jwtSecret, purchaseAirtimeHandler(db, airtimeProvider)))
	mux.HandleFunc("/api/v1/transactions/", getTransactionHandler(db))
	mux.HandleFunc("/api/v1/auth/register", registerHandler(db))
	mux.HandleFunc("/api/v1/auth/login", loginHandler(db, jwtSecret))
	mux.HandleFunc("/api/v1/user/transactions", requireAuthMiddleware(jwtSecret, userTransactionsHandler(db)))
	mux.HandleFunc("/api/v1/wallet", requireAuthMiddleware(jwtSecret, getWalletHandler(db)))

	// 5. Configure HTTP server with safety timeouts and CORS
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("QuickTop server running on port %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed to start: %v", err)
	}
}
