package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// HealthResponse defines the structure of our JSON response
type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// healthHandler processes requests to /health
func healthHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Set the response header to tell the client we are returning JSON
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// 3. Prepare the response data
	res := HealthResponse{
		Status:  "ok",
		Message: "QuickTop API is running",
	}

	// 4. Encode the struct into JSON and write it directly to the response writer
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("Failed to encode health response: %v", err)
	}
}

func main() {
	// Create a new router (ServeMux)
	mux := http.NewServeMux()

	// Register our route
	mux.HandleFunc("/health", healthHandler)

	// Configure the server with timeouts for resilience
	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	fmt.Println("Server is running on http://localhost:8080")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
