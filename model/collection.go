package model

import (
	"fmt"
)

// maxCollectionTransactions bounds one post, which a page closing flushes
// several transactions into.
const maxCollectionTransactions = 32

// CollectRoute is the route of the front end a client posts transactions to,
// relative to Options.Path.
const CollectRoute = "/collect"

// CollectLimit is the body a collect request may carry. A beacon is capped at
// 64 KiB by the browser, and the route refuses more than that.
const CollectLimit = 64 << 10

// Collection is the batch of transactions a client posts to the collect
// route.
type Collection struct {
	// Transactions are the units of work the client recorded, in the order
	// it recorded them.
	Transactions []Transaction `json:"transactions"`
}

// Validate reports whether the collection is one the recorder will read.
// Every failure wraps ErrInvalidTransaction.
func (c Collection) Validate() error {
	switch {
	case len(c.Transactions) == 0:
		return fmt.Errorf("%w: a collection carries at least one transaction", ErrInvalidTransaction)
	case len(c.Transactions) > maxCollectionTransactions:
		return fmt.Errorf("%w: %d transactions is past the %d one post may carry", ErrInvalidTransaction, len(c.Transactions), maxCollectionTransactions)
	}
	return nil
}
