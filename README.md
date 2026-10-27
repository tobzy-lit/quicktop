# QuickTop Backend

QuickTop is an airtime and data subscription backend for the Nigerian market built using Go and PostgreSQL.

## Prerequisites
- Go 1.22+
- PostgreSQL 16
- Git & curl

## Environment Configuration
Copy `.env.example` to `.env` and configure your local connection string:
```bash
cp .env.example .env
```

## Endpoints
- `GET /health` : Liveness check (checks server process)
- `GET /ready` : Readiness probe (checks PostgreSQL connectivity)
- `GET /api/v1/networks` : Lists supported Nigerian telcos (MTN, Airtel, Glo, 9mobile)
- `POST /api/v1/airtime/validate` : Validates recipient number, network, and amount

## Database Migrations
Schema migrations are located in `migrations/`. Stored amounts use integer Kobo (1 Naira = 100 Kobo).

## How to Run
```bash
go run .
```

## How to Test
Runs unit and PostgreSQL integration tests:
```bash
go test -v ./...
```
