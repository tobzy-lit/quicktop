package main

import (
	"errors"
	"regexp"
	"strings"
)

// Supported telco networks
var supportedNetworks = map[string]bool{
	"MTN":     true,
	"AIRTEL":  true,
	"GLO":     true,
	"9MOBILE": true,
}

// Regex matching standard 11-digit or +234 Nigerian mobile formats
var nigerianPhoneRegex = regexp.MustCompile(`^(?:\+234|0)[789][01]\d{8}$`)

// AirtimeValidationRequest defines the incoming JSON structure
type AirtimeValidationRequest struct {
	Network     string `json:"network"`
	PhoneNumber string `json:"phone_number"`
	Amount      int    `json:"amount"`
}

// Validate checks the fields of an AirtimeValidationRequest
func (req *AirtimeValidationRequest) Validate() error {
	// 1. Validate Network
	trimmedNetwork := strings.ToUpper(strings.TrimSpace(req.Network))
	if trimmedNetwork == "" {
		return errors.New("network is required")
	}
	if !supportedNetworks[trimmedNetwork] {
		return errors.New("unsupported network; allowed: MTN, Airtel, Glo, 9mobile")
	}

	// 2. Validate Phone Number
	trimmedPhone := strings.TrimSpace(req.PhoneNumber)
	if trimmedPhone == "" {
		return errors.New("phone_number is required")
	}
	if !nigerianPhoneRegex.MatchString(trimmedPhone) {
		return errors.New("invalid Nigerian phone number format")
	}

	// 3. Validate Amount
	if req.Amount <= 0 {
		return errors.New("amount must be a positive whole number")
	}
	if req.Amount < 50 || req.Amount > 50000 {
		return errors.New("amount must be between 50 and 50,000 Naira")
	}

	return nil
}
