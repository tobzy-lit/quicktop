package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
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

// ValidationSuccessResponse is returned when the airtime request passes all checks
type ValidationSuccessResponse struct {
	Status  string                   `json:"status"`
	Message string                   `json:"message"`
	Data    AirtimeValidationRequest `json:"data"`
}

// ErrorResponse defines standard client/server error responses
type ErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// writeJSONHelper simplifies sending JSON responses with the correct status code
func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

// healthHandler processes GET /health
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:  "ok",
		Message: "QuickTop API is running",
	})
}

// networksHandler processes GET /api/v1/networks
func networksHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, NetworksResponse{
		Status:   "ok",
		Networks: []string{"MTN", "Airtel", "Glo", "9mobile"},
	})
}

// validateAirtimeHandler processes POST /api/v1/airtime/validate
func validateAirtimeHandler(w http.ResponseWriter, r *http.Request) {
	// 1. Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. Validate Content-Type header
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, ErrorResponse{
			Status: "error",
			Error:  "Content-Type must be application/json",
		})
		return
	}

	// 3. Limit request body size to 1 MB (1048576 bytes)
	r.Body = http.MaxBytesReader(w, r.Body, 1048576)

	// 4. Decode JSON with strict validation
	var req AirtimeValidationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeErr *json.UnmarshalTypeError

		switch {
		case errors.As(err, &syntaxErr):
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Malformed JSON syntax in request body",
			})
		case errors.As(err, &unmarshalTypeErr):
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Invalid field type in request body",
			})
		case strings.HasPrefix(err.Error(), "json: unknown field"):
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  err.Error(),
			})
		case errors.Is(err, io.EOF):
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Request body cannot be empty",
			})
		case err.Error() == "http: request body too large":
			writeJSON(w, http.StatusRequestEntityTooLarge, ErrorResponse{
				Status: "error",
				Error:  "Request body exceeds 1MB limit",
			})
		default:
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Invalid request body",
			})
		}
		return
	}

	// 5. Ensure there is no extraneous data after the JSON object
	if decoder.More() {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Status: "error",
			Error:  "Request body must contain only a single JSON object",
		})
		return
	}

	// 6. Run business validation from validators.go
	if err := req.Validate(); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
			Status: "error",
			Error:  err.Error(),
		})
		return
	}

	// 7. Request is valid!
	writeJSON(w, http.StatusOK, ValidationSuccessResponse{
		Status:  "ok",
		Message: "Airtime request validation successful",
		Data:    req,
	})
}
