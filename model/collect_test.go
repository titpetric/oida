package model

import (
	"context"
	"testing"
)

func TestCollect(t *testing.T) {
	recorder := &countingRecorder{}
	collection := Collection{Transactions: []Transaction{browserTransaction(), browserTransaction()}}

	receipt, err := Collect(context.Background(), recorder, collection)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if recorder.recorded != 2 || len(receipt.Transactions) != 2 {
		t.Fatalf("recorded %d and answered %d of 2", recorder.recorded, len(receipt.Transactions))
	}
	if receipt.Transactions[0].TraceParent != exampleParent {
		t.Fatalf("the receipt reads %q", receipt.Transactions[0].TraceParent)
	}

	recorder.refuse = true
	receipt, err = Collect(context.Background(), recorder, collection)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if receipt.Transactions[0].Error == "" {
		t.Fatal("a refused transaction came back without a reason")
	}

	if _, err := Collect(context.Background(), recorder, Collection{}); err == nil {
		t.Fatal("an empty collection was accepted")
	}
}
