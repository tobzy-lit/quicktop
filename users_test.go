package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	rawPassword := "SecurePass123!"

	hash, err := hashPassword(rawPassword)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	if hash == rawPassword {
		t.Fatal("password hash must not match plaintext password")
	}

	if !checkPasswordHash(rawPassword, hash) {
		t.Errorf("expected valid password check to pass")
	}

	if checkPasswordHash("WrongPassword!", hash) {
		t.Errorf("expected wrong password check to fail")
	}
}

func TestCreateUser_Integration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://quicktop_user:quicktop_pass_local@localhost:5432/quicktop_dev?sslmode=disable"
	}

	db, err := initDB(dbURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	uniqueEmail := fmt.Sprintf("testuser_%d@example.com", time.Now().UnixNano())
	hash, err := hashPassword("password123")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	user := &User{
		Name:         "Integration Test User",
		Email:        uniqueEmail,
		PasswordHash: hash,
	}

	// 1. First insert should succeed
	if err := createUser(ctx, db, user); err != nil {
		t.Fatalf("createUser failed: %v", err)
	}

	if user.ID <= 0 {
		t.Errorf("expected generated ID > 0, got %d", user.ID)
	}
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Errorf("expected populated timestamps, got created: %v, updated: %v", user.CreatedAt, user.UpdatedAt)
	}

	// 2. Second insert with identical email must return ErrDuplicateEmail
	duplicateUser := &User{
		Name:         "Duplicate User",
		Email:        uniqueEmail,
		PasswordHash: hash,
	}

	dupErr := createUser(ctx, db, duplicateUser)
	if dupErr == nil {
		t.Fatalf("expected duplicate email error, got nil")
	}

	if !errors.Is(dupErr, ErrDuplicateEmail) {
		t.Errorf("expected ErrDuplicateEmail, got: %v", dupErr)
	}
}

func TestJWT_GenerateAndVerify(t *testing.T) {
	secret := "test-secret-key-12345"

	// 1. Successful generation and verification
	tokenStr, err := generateToken(42, "testuser@example.com", secret, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := verifyToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("failed to verify valid token: %v", err)
	}

	if claims.UserID != 42 {
		t.Errorf("expected UserID 42, got %d", claims.UserID)
	}
	if claims.Email != "testuser@example.com" {
		t.Errorf("expected email testuser@example.com, got %s", claims.Email)
	}

	// 2. Reject verification with wrong secret
	_, err = verifyToken(tokenStr, "wrong-secret-key")
	if err == nil {
		t.Errorf("expected verification failure with wrong secret, got nil")
	}

	// 3. Reject expired token
	expiredTokenStr, err := generateToken(42, "testuser@example.com", secret, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	_, err = verifyToken(expiredTokenStr, secret)
	if err == nil {
		t.Errorf("expected verification failure for expired token, got nil")
	}
}
