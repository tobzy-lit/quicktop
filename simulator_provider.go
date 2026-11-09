package main

import (
	"context"
	"fmt"
	"time"
)

type SimulatorProvider struct{}

func NewSimulatorProvider() *SimulatorProvider {
	return &SimulatorProvider{}
}

func (s *SimulatorProvider) Name() string {
	return "simulator"
}

func (s *SimulatorProvider) DispatchAirtime(ctx context.Context, tx *Transaction) (*ProviderResult, error) {
	// Generate a simulated provider reference
	mockRef := fmt.Sprintf("SIM-%d", time.Now().UnixNano())

	return &ProviderResult{
		ExternalReference: mockRef,
		Status:            "PENDING",
		ResponseCode:      "00",
		RawResponse:       `{"status":"simulated_queued"}`,
	}, nil
}
