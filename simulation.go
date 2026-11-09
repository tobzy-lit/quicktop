package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"
)

// dispatchAirtimeTransaction executes airtime delivery in the background using the active AirtimeProvider
func dispatchAirtimeTransaction(db *sql.DB, provider AirtimeProvider, tx *Transaction) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		// 1. Move to PROCESSING
		if err := updateTransactionStatus(ctx, db, tx.Reference, StatusPending, StatusProcessing); err != nil {
			log.Printf("[%s] Failed moving %s to PROCESSING: %v", provider.Name(), tx.Reference, err)
			return
		}
		log.Printf("[%s] Transaction %s moved to PROCESSING", provider.Name(), tx.Reference)

		// 2. Dispatch to the active provider (VTpass or Simulator)
		result, err := provider.DispatchAirtime(ctx, tx)
		if err != nil {
			log.Printf("[%s] Provider dispatch error for %s: %v", provider.Name(), tx.Reference, err)
			_ = updateTransactionStatus(ctx, db, tx.Reference, StatusProcessing, StatusFailed)
			return
		}

		// 3. Resolve status based on provider response
		finalStatus := StatusSuccess
		if result.Status == "FAILED" || strings.HasSuffix(tx.PhoneNumber, "0000") {
			finalStatus = StatusFailed
		}

		// 4. Update PostgreSQL to terminal state
		if err := updateTransactionStatus(ctx, db, tx.Reference, StatusProcessing, finalStatus); err != nil {
			log.Printf("[%s] Failed finalizing %s to %s: %v", provider.Name(), tx.Reference, finalStatus, err)
			return
		}

		log.Printf("[%s] Transaction %s finalized as %s (ExternalRef: %s)", provider.Name(), tx.Reference, finalStatus, result.ExternalReference)
	}()
}
