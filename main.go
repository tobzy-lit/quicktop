package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/api/v1/networks", networksHandler)
	mux.HandleFunc("/api/v1/airtime/validate", validateAirtimeHandler)

	// Server configuration
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
