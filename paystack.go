package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type PaystackClient struct {
	SecretKey  string
	BaseURL    string
	HTTPClient *http.Client
}

func NewPaystackClient() *PaystackClient {
	baseURL := os.Getenv("PAYSTACK_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.paystack.co"
	}
	return &PaystackClient{
		SecretKey: os.Getenv("PAYSTACK_SECRET_KEY"),
		BaseURL:   baseURL,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type paystackInitRequest struct {
	Email       string `json:"email"`
	Amount      int64  `json:"amount"` // In Kobo
	Reference   string `json:"reference"`
	CallbackURL string `json:"callback_url,omitempty"`
}

type PaystackInitData struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

type paystackInitResponse struct {
	Status  bool             `json:"status"`
	Message string           `json:"message"`
	Data    PaystackInitData `json:"data"`
}

type PaystackVerifyData struct {
	Status          string `json:"status"` // "success", "failed", "abandoned"
	Reference       string `json:"reference"`
	Amount          int64  `json:"amount"` // In Kobo
	GatewayResponse string `json:"gateway_response"`
}

type paystackVerifyResponse struct {
	Status  bool               `json:"status"`
	Message string             `json:"message"`
	Data    PaystackVerifyData `json:"data"`
}

// InitializeTransaction requests a checkout URL from Paystack
func (c *PaystackClient) InitializeTransaction(ctx context.Context, email string, amountKobo int64, reference, callbackURL string) (*PaystackInitData, error) {
	if c.SecretKey == "" {
		return nil, errors.New("PAYSTACK_SECRET_KEY is not configured")
	}

	reqBody := paystackInitRequest{
		Email:       email,
		Amount:      amountKobo,
		Reference:   reference,
		CallbackURL: callbackURL,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal paystack request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/transaction/initialize", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read paystack response: %w", err)
	}

	var parsed paystackInitResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode paystack response: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !parsed.Status {
		return nil, fmt.Errorf("paystack initialization rejected: %s", parsed.Message)
	}

	return &parsed.Data, nil
}

// VerifyTransaction queries Paystack directly to confirm transaction settlement
func (c *PaystackClient) VerifyTransaction(ctx context.Context, reference string) (*PaystackVerifyData, error) {
	if c.SecretKey == "" {
		return nil, errors.New("PAYSTACK_SECRET_KEY is not configured")
	}

	url := fmt.Sprintf("%s/transaction/verify/%s", c.BaseURL, reference)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.SecretKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paystack verify request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read verify response: %w", err)
	}

	var parsed paystackVerifyResponse
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode verify response: %w", err)
	}

	if resp.StatusCode != http.StatusOK || !parsed.Status {
		return nil, fmt.Errorf("paystack verification failed: %s", parsed.Message)
	}

	return &parsed.Data, nil
}

// VerifyPaystackSignature validates the HMAC-SHA512 signature from X-Paystack-Signature
func VerifyPaystackSignature(rawBody []byte, signatureHeader, secretKey string) bool {
	if signatureHeader == "" || secretKey == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secretKey))
	mac.Write(rawBody)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expectedMAC), []byte(signatureHeader))
}
