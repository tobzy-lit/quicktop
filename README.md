# QuickTop Backend — Milestone 1

QuickTop is an airtime and data subscription backend for the Nigerian market built using Go.

## Endpoints
- GET /health : Health check
- GET /api/v1/networks : List supported Nigerian telcos
- POST /api/v1/airtime/validate : Validate phone format, network, and amount

## How to Run
go run .

## How to Test
go test -v ./...
