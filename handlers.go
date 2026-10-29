package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
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

// ReadinessResponse reports whether dependencies are reachable
type ReadinessResponse struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

// readyHandler checks external dependencies like PostgreSQL
func readyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Ping the database with a quick 2-second timeout
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, ReadinessResponse{
				Status:   "error",
				Database: "unreachable",
			})
			return
		}

		writeJSON(w, http.StatusOK, ReadinessResponse{
			Status:   "ok",
			Database: "connected",
		})
	}
}

// AirtimePurchaseResponse represents the response after purchase initiation
type AirtimePurchaseResponse struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	Reference   string `json:"reference"`
	Network     string `json:"network"`
	PhoneNumber string `json:"phone_number"`
	AmountNaira int    `json:"amount_naira"`
	AmountKobo  int64  `json:"amount_kobo"`
	TxStatus    string `json:"transaction_status"`
}

// purchaseAirtimeHandler handles POST /api/v1/airtime/purchase
func purchaseAirtimeHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Method check
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 2. Content-Type check
		contentType := r.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "application/json") {
			writeJSON(w, http.StatusUnsupportedMediaType, ErrorResponse{
				Status: "error",
				Error:  "Content-Type must be application/json",
			})
			return
		}

		// 3. Limit body size to 1 MB
		r.Body = http.MaxBytesReader(w, r.Body, 1048576)

		// 4. Strict JSON decoding
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

		if decoder.More() {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Request body must contain only a single JSON object",
			})
			return
		}

		// 5. Business rules validation
		if err := req.Validate(); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, ErrorResponse{
				Status: "error",
				Error:  err.Error(),
			})
			return
		}

		// 6. Generate reference
		ref, err := generateTransactionReference()
		if err != nil {
			log.Printf("Reference generation failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Status: "error",
				Error:  "Internal system error creating purchase reference",
			})
			return
		}

		// 7. Persist transaction with PENDING status and integer Kobo
		cleanNetwork := strings.ToUpper(strings.TrimSpace(req.Network))
		cleanPhone := strings.TrimSpace(req.PhoneNumber)
		amountKobo := int64(req.Amount) * 100

		txRecord := &Transaction{
			Reference:   ref,
			Network:     cleanNetwork,
			PhoneNumber: cleanPhone,
			AmountKobo:  amountKobo,
			Status:      StatusPending,
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := createTransaction(ctx, db, txRecord); err != nil {
			log.Printf("Failed to insert purchase transaction: %v", err)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Status: "error",
				Error:  "Failed to record transaction",
			})
			return
		}

		// Trigger background simulation
		simulateAirtimeProcessing(db, txRecord.Reference, txRecord.PhoneNumber)

		// 8. Return 201 Created with initial state
		writeJSON(w, http.StatusCreated, AirtimePurchaseResponse{
			Status:      "ok",
			Message:     "Airtime purchase initiated successfully",
			Reference:   txRecord.Reference,
			Network:     txRecord.Network,
			PhoneNumber: txRecord.PhoneNumber,
			AmountNaira: req.Amount,
			AmountKobo:  txRecord.AmountKobo,
			TxStatus:    txRecord.Status,
		})
	}
}

// TransactionDetailsResponse formats the public response for a transaction query
type TransactionDetailsResponse struct {
	Status      string    `json:"status"`
	Reference   string    `json:"reference"`
	Network     string    `json:"network"`
	PhoneNumber string    `json:"phone_number"`
	AmountNaira int64     `json:"amount_naira"`
	AmountKobo  int64     `json:"amount_kobo"`
	TxStatus    string    `json:"transaction_status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// getTransactionHandler processes GET /api/v1/transactions/{reference}
func getTransactionHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Only allow GET
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// 2. Extract reference from URL path (/api/v1/transactions/{reference})
		prefix := "/api/v1/transactions/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}

		reference := strings.TrimPrefix(r.URL.Path, prefix)
		reference = strings.TrimSpace(reference)

		if reference == "" {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Status: "error",
				Error:  "Transaction reference is required",
			})
			return
		}

		// 3. Query PostgreSQL
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		tx, err := getTransactionByReference(ctx, db, reference)
		if err != nil {
			if errors.Is(err, ErrTransactionNotFound) {
				writeJSON(w, http.StatusNotFound, ErrorResponse{
					Status: "error",
					Error:  "Transaction not found",
				})
				return
			}

			log.Printf("Error retrieving transaction %s: %v", reference, err)
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Status: "error",
				Error:  "Internal server error",
			})
			return
		}

		// 4. Return transaction details (convert Kobo to Naira for display)
		writeJSON(w, http.StatusOK, TransactionDetailsResponse{
			Status:      "ok",
			Reference:   tx.Reference,
			Network:     tx.Network,
			PhoneNumber: tx.PhoneNumber,
			AmountNaira: tx.AmountKobo / 100,
			AmountKobo:  tx.AmountKobo,
			TxStatus:    tx.Status,
			CreatedAt:   tx.CreatedAt,
			UpdatedAt:   tx.UpdatedAt,
		})
	}
}
