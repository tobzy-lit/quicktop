package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	minTopupKobo int64 = 10000    // ₦100.00 minimum
	maxTopupKobo int64 = 50000000 // ₦500,000.00 maximum
)

type InitializeTopupRequest struct {
	AmountNaira float64 `json:"amount_naira"`
	CallbackURL string  `json:"callback_url,omitempty"`
}

type VerifyTopupRequest struct {
	Reference string `json:"reference"`
}

type paystackWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		Reference string `json:"reference"`
		Amount    int64  `json:"amount"`
		Status    string `json:"status"`
		ID        int64  `json:"id"`
	} `json:"data"`
}

func generateTopupReference() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("TOPUP-QT-%d-%s", time.Now().Unix(), strings.ToUpper(hex.EncodeToString(b))), nil
}

// settleTopupTransaction atomically marks a pending topup as SUCCESS and credits the user's wallet.
// It is idempotent: calling it multiple times for an already-settled reference is a safe no-op.
func settleTopupTransaction(ctx context.Context, db *sql.DB, reference string, paidAmountKobo int64, gatewayRef string) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to begin tx: %w", err)
	}
	defer tx.Rollback()

	topup, err := GetTopupByReferenceForUpdate(ctx, tx, reference)
	if err != nil {
		return false, err
	}

	// Idempotency guard: already settled
	if topup.Status == "SUCCESS" {
		return false, nil
	}

	// Amount integrity check: reject if gateway settled less than expected
	if paidAmountKobo < topup.AmountKobo {
		_ = UpdateTopupStatus(ctx, tx, reference, "FAILED", gatewayRef)
		_ = tx.Commit()
		return false, fmt.Errorf("amount mismatch: expected %d kobo, paid %d kobo", topup.AmountKobo, paidAmountKobo)
	}

	if err := UpdateTopupStatus(ctx, tx, reference, "SUCCESS", gatewayRef); err != nil {
		return false, err
	}

	desc := fmt.Sprintf("Wallet funding via %s (%s)", topup.Gateway, reference)
	if _, err := CreditWallet(ctx, tx, topup.UserID, topup.AmountKobo, reference, desc); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit topup settlement: %w", err)
	}

	return true, nil
}

// initializeTopupHandler starts a wallet funding session and returns a Paystack checkout URL
func initializeTopupHandler(db *sql.DB, paystack *PaystackClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"status": "error",
				"error":  "Method not allowed",
			})
			return
		}

		userID, ok := getUserIDFromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Unauthorized",
			})
			return
		}

		var req InitializeTopupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error",
				"error":  "Invalid JSON request body",
			})
			return
		}

		amountKobo := int64(req.AmountNaira * 100)
		if amountKobo < minTopupKobo || amountKobo > maxTopupKobo {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error",
				"error":  "Amount must be between ₦100 and ₦500,000",
			})
			return
		}

		user, err := getUserByID(r.Context(), db, userID)
		if err != nil {
			log.Printf("Topup init failed to load user %d: %v", userID, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error",
				"error":  "Failed to load user profile",
			})
			return
		}

		ref, err := generateTopupReference()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error",
				"error":  "Failed to generate payment reference",
			})
			return
		}

		topup := &WalletTopup{
			UserID:     userID,
			Reference:  ref,
			AmountKobo: amountKobo,
			Gateway:    "PAYSTACK",
		}
		if err := CreateTopup(r.Context(), db, topup); err != nil {
			log.Printf("Failed to persist topup intent: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error",
				"error":  "Failed to initialize funding record",
			})
			return
		}

		initData, err := paystack.InitializeTransaction(r.Context(), user.Email, amountKobo, ref, req.CallbackURL)
		if err != nil {
			log.Printf("Paystack initialize error for ref %s: %v", ref, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"status": "error",
				"error":  "Payment gateway initialization failed",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":            "ok",
			"reference":         initData.Reference,
			"authorization_url": initData.AuthorizationURL,
			"access_code":       initData.AccessCode,
			"amount_kobo":       amountKobo,
		})
	}
}

// paystackWebhookHandler processes signed webhook events from Paystack
func paystackWebhookHandler(db *sql.DB, paystackSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Limit payload size to 1MB to prevent memory exhaustion attacks
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		signature := r.Header.Get("X-Paystack-Signature")
		if !VerifyPaystackSignature(bodyBytes, signature, paystackSecret) {
			log.Printf("[Security] Rejected Paystack webhook with invalid HMAC signature")
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Invalid webhook signature",
			})
			return
		}

		var event paystackWebhookPayload
		if err := json.Unmarshal(bodyBytes, &event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Acknowledge non-charge events cleanly
		if event.Event != "charge.success" || event.Data.Status != "success" {
			w.WriteHeader(http.StatusOK)
			return
		}

		gatewayRef := fmt.Sprintf("%d", event.Data.ID)
		credited, err := settleTopupTransaction(r.Context(), db, event.Data.Reference, event.Data.Amount, gatewayRef)
		if err != nil {
			if errors.Is(err, ErrTopupNotFound) {
				log.Printf("[Webhook] Unknown topup reference: %s", event.Data.Reference)
				w.WriteHeader(http.StatusOK)
				return
			}
			log.Printf("[Webhook] Settlement error for %s: %v", event.Data.Reference, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if credited {
			log.Printf("[Webhook] Credited wallet for reference %s (%d kobo)", event.Data.Reference, event.Data.Amount)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// verifyTopupHandler lets authenticated clients confirm a pending top-up directly
func verifyTopupHandler(db *sql.DB, paystack *PaystackClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
				"status": "error",
				"error":  "Method not allowed",
			})
			return
		}

		if _, ok := getUserIDFromContext(r.Context()); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"status": "error",
				"error":  "Unauthorized",
			})
			return
		}

		var req VerifyTopupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Reference) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"status": "error",
				"error":  "Valid payment reference is required",
			})
			return
		}

		ref := strings.TrimSpace(req.Reference)
		verifyData, err := paystack.VerifyTransaction(r.Context(), ref)
		if err != nil {
			log.Printf("Manual verify failed for %s: %v", ref, err)
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"status": "error",
				"error":  "Unable to verify payment with gateway",
			})
			return
		}

		if verifyData.Status != "success" {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{
				"status":          "pending",
				"gateway_status":  verifyData.Status,
				"gateway_message": verifyData.GatewayResponse,
			})
			return
		}

		_, err = settleTopupTransaction(r.Context(), db, ref, verifyData.Amount, verifyData.Reference)
		if err != nil {
			if errors.Is(err, ErrTopupNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"status": "error",
					"error":  "Topup reference not found",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"status": "error",
				"error":  "Failed to settle wallet topup",
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"reference":   ref,
			"amount_kobo": verifyData.Amount,
			"message":     "Wallet funded successfully",
		})
	}
}
