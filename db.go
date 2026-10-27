package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // Registers pgx driver with database/sql
)

// initDB establishes a verified connection pool to PostgreSQL using the provided connection string.
func initDB(connStr string) (*sql.DB, error) {
	if connStr == "" {
		return nil, fmt.Errorf("database connection string cannot be empty")
	}

	// 1. Initialize the sql.DB abstraction using the pgx driver
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database handle: %w", err)
	}

	// 2. Configure connection pool limits
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 3. Verify the connection with a 5-second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL database")
	return db, nil
}
