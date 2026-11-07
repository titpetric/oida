package model

// Receipt is the answer to a Collection: one Collected per posted
// transaction, in the order they arrived.
type Receipt struct {
	// Transactions are the answers, one per posted transaction.
	Transactions []Collected `json:"transactions"`
}
