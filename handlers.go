package main

import (
	"encoding/json"
	"log"
	"net/http"
)

// HealthResponse defines the structure of our health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NetworksResponse defines the structure of the networks response
type NetworksResponse struct {
	Status   string   `json:"status"`
	Networks []string `json:"networks"`
}

// healthHandler processes GET /health
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	res := HealthResponse{
		Status:  "ok",
		Message: "QuickTop API is running",
	}

	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("Failed to encode health response: %v", err)
	}
}

// networksHandler processes GET /api/v1/networks
func networksHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Only allow GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Set JSON header
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// 3. Define the supported networks slice
	res := NetworksResponse{
		Status: "ok",
		Networks: []string{
			"MTN",
			"Airtel",
			"Glo",
			"9mobile",
		},
	}

	// 4. Encode and return JSON
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("Failed to encode networks response: %v", err)
	}
}
