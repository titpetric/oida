package model

import (
	"context"
)

// Collect records every transaction of a collection through a recorder and
// returns the receipt, one answer per transaction.
//
// A transaction the recorder refuses leaves its reason in that answer, so one
// bad entry costs the rest nothing. The front end and the tracer both go
// through this, so the route and the method record alike.
func Collect(ctx context.Context, recorder Recorder, collection Collection) (Receipt, error) {
	if err := collection.Validate(); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{Transactions: make([]Collected, 0, len(collection.Transactions))}
	for _, tx := range collection.Transactions {
		collected, err := recorder.Record(ctx, tx)
		if err != nil {
			collected.Error = err.Error()
		}
		receipt.Transactions = append(receipt.Transactions, collected)
	}
	return receipt, nil
}
