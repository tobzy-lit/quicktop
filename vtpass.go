package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type VTpassClient struct {
	BaseURL    string
	APIKey     string
	SecretKey  string
	PublicKey  string
	HTTPClient *http.Client
}

func NewVTpassClient(baseURL, apiKey, secretKey, publicKey string) *VTpassClient {
	return &VTpassClient{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		APIKey:    apiKey,
		SecretKey: secretKey,
		PublicKey: publicKey,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (v *VTpassClient) Name() string {
	return "vtpass"
}

// mapNetworkToServiceID maps QuickTop network names to VTpass serviceID
func (v *VTpassClient) mapNetworkToServiceID(network string) string {
	switch strings.ToUpper(network) {
	case "MTN":
		return "mtn"
	case "AIRTEL":
		return "airtel"
	case "GLO":
		return "glo"
	case "9MOBILE":
		return "etisalat"
	default:
		return strings.ToLower(network)
	}
}

// generateRequestID formats: YYYYMMDDHHMM[uniqueSuffix]
func (v *VTpassClient) generateRequestID() string {
	return fmt.Sprintf("%s%d", time.Now().Format("200601021504"), time.Now().UnixNano()%100000)
}

func (v *VTpassClient) DispatchAirtime(ctx context.Context, tx *Transaction) (*ProviderResult, error) {
	serviceID := v.mapNetworkToServiceID(tx.Network)
	amountNaira := tx.AmountKobo / 100 // Convert kobo to Naira for VTpass payload

	payload := map[string]interface{}{
		"request_id":     v.generateRequestID(),
		"serviceID":      serviceID,
		"billersCode":    tx.PhoneNumber,
		"variation_code": "airtime",
		"amount":         amountNaira,
		"phone":          tx.PhoneNumber,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode vtpass payload: %w", err)
	}

	url := fmt.Sprintf("%s/pay", v.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create vtpass request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", v.APIKey)
	req.Header.Set("secret-key", v.SecretKey)

	resp, err := v.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vtpass network error: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read vtpass response: %w", err)
	}

	var parsed struct {
		Code          string `json:"code"`
		ResponseDesc  string `json:"response_description"`
		RequestId     string `json:"requestId"`
		TransactionId string `json:"purchased_code"`
		Content       struct {
			Transactions struct {
				Status string `json:"status"`
			} `json:"transactions"`
		} `json:"content"`
	}

	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return &ProviderResult{
			Status:       "PENDING",
			RawResponse:  string(respBytes),
			ResponseCode: fmt.Sprintf("%d", resp.StatusCode),
		}, nil
	}

	result := &ProviderResult{
		ExternalReference: parsed.RequestId,
		RawResponse:       string(respBytes),
		ResponseCode:      parsed.Code,
	}

	if parsed.Code == "000" {
		result.Status = "SUCCESS"
	} else if parsed.Code == "099" || parsed.Code == "016" {
		result.Status = "PENDING"
	} else {
		result.Status = "FAILED"
	}

	return result, nil
}
