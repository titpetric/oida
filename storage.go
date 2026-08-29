package oida

import "github.com/titpetric/oida/model"

// Storage retains completed traces. Implementations must be safe for concurrent
// use: the tracer writes from request goroutines and reads from the debug front
// end at the same time.
//
// Two implementations ship with this package: StorageMemory, a bounded ring
// buffer, and StorageDisk, a bounded folder of JSON documents.
type Storage = model.Storage
