package model

import (
	"errors"
	"testing"
)

func TestCollection(t *testing.T) {
	if err := (Collection{Transactions: []Transaction{browserTransaction()}}).Validate(); err != nil {
		t.Fatalf("a one transaction collection is invalid: %v", err)
	}
}

func TestCollection_Validate(t *testing.T) {
	if err := (Collection{}).Validate(); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("an empty collection returned %v", err)
	}
	if err := (Collection{Transactions: make([]Transaction, maxCollectionTransactions)}).Validate(); err != nil {
		t.Fatalf("a full collection is invalid: %v", err)
	}
	full := Collection{Transactions: make([]Transaction, maxCollectionTransactions+1)}
	if err := full.Validate(); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("an oversized collection returned %v", err)
	}
}
