package model

import (
	"testing"
)

func TestReceipt(t *testing.T) {
	receipt := Receipt{Transactions: []Collected{NewCollected("01K0000000000000000000000A", TraceContext{})}}
	if len(receipt.Transactions) != 1 || receipt.Transactions[0].ID != "01K0000000000000000000000A" {
		t.Fatalf("the receipt holds %+v", receipt.Transactions)
	}
}
