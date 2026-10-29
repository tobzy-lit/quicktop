package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"
)

// simulateAirtimeProcessing simulates an asynchronous call to a telco provider
// It transitions PENDING -> PROCESSING -> SUCCESS or FAILED
func simulateAirtimeProcessing(db *sql.DB, reference, phoneNumber string) {
	// Background execution detached from the initial client HTTP request
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// 1. Step: Transition to PROCESSING
		time.Sleep(500 * time.Millisecond) // Simulated network latency
		if err := updateTransactionStatus(ctx, db, reference, StatusPending, StatusProcessing); err != nil {
			log.Printf("[Simulator] Failed moving %s to PROCESSING: %v", reference, err)
			return
		}
		log.Printf("[Simulator] Transaction %s moved to PROCESSING", reference)

		// 2. Step: Determine outcome deterministically
		time.Sleep(1 * time.Second) // Simulated telco execution time
		finalStatus := StatusSuccess
		if strings.HasSuffix(phoneNumber, "0000") {
			finalStatus = StatusFailed
		}

		// 3. Step: Transition to terminal state (SUCCESS or FAILED)
		if err := updateTransactionStatus(ctx, db, reference, StatusProcessing, finalStatus); err != nil {
			log.Printf("[Simulator] Failed finalizing %s to %s: %v", reference, finalStatus, err)
			return
		}
		log.Printf("[Simulator] Transaction %s finalized as %s", reference, finalStatus)
	}()
}
