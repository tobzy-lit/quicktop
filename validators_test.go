package main

import "testing"

func TestAirtimeValidation(t *testing.T) {
	tests := []struct {
		name        string
		request     AirtimeValidationRequest
		shouldError bool
	}{
		{
			name: "Valid local Nigerian format",
			request: AirtimeValidationRequest{
				Network:     "MTN",
				PhoneNumber: "08012345678",
				Amount:      1000,
			},
			shouldError: false,
		},
		{
			name: "Valid international Nigerian format",
			request: AirtimeValidationRequest{
				Network:     "Airtel",
				PhoneNumber: "+2348012345678",
				Amount:      500,
			},
			shouldError: false,
		},
		{
			name: "Unsupported network",
			request: AirtimeValidationRequest{
				Network:     "Vodafone",
				PhoneNumber: "08012345678",
				Amount:      1000,
			},
			shouldError: true,
		},
		{
			name: "Invalid short phone number",
			request: AirtimeValidationRequest{
				Network:     "Glo",
				PhoneNumber: "080123",
				Amount:      1000,
			},
			shouldError: true,
		},
		{
			name: "Zero amount",
			request: AirtimeValidationRequest{
				Network:     "9mobile",
				PhoneNumber: "08012345678",
				Amount:      0,
			},
			shouldError: true,
		},
		{
			name: "Amount exceeding upper limit",
			request: AirtimeValidationRequest{
				Network:     "MTN",
				PhoneNumber: "08012345678",
				Amount:      100000,
			},
			shouldError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.request.Validate()
			if tc.shouldError && err == nil {
				t.Errorf("expected validation to fail for case '%s', but it passed", tc.name)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("expected validation to pass for case '%s', but got error: %v", tc.name, err)
			}
		})
	}
}
