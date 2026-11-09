package main

import (
	"context"
)

// ProviderResult represents the normalized response from any VTU aggregator
type ProviderResult struct {
	ExternalReference string
	Status            string // "SUCCESS", "PENDING", "FAILED"
	RawResponse       string
	ResponseCode      string
}

// AirtimeProvider abstracts the third-party airtime dispatch logic
type AirtimeProvider interface {
	DispatchAirtime(ctx context.Context, tx *Transaction) (*ProviderResult, error)
	Name() string
}
