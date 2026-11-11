package model

import (
	"errors"
	"testing"
)

func TestErrInvalidOptions(t *testing.T) {
	for _, err := range []error{ErrInvalidPath, ErrInvalidSampleRate, invalidOption("ring_buffer_size", "must not be negative")} {
		if !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%v does not wrap ErrInvalidOptions", err)
		}
	}
}

func TestErrInvalidTraceParent(t *testing.T) {
	_, err := ParseTraceParent("nonsense")
	if !errors.Is(err, ErrInvalidTraceParent) {
		t.Fatalf("parse returned %v, want ErrInvalidTraceParent", err)
	}
}

func TestErrInvalidTransaction(t *testing.T) {
	if err := (Transaction{}).Validate(); !errors.Is(err, ErrInvalidTransaction) {
		t.Fatalf("validate returned %v, want ErrInvalidTransaction", err)
	}
}

func TestInvalidOption(t *testing.T) {
	err := invalidOption("top_requests", "must not be negative")
	if got, want := err.Error(), "oida: invalid options: top_requests must not be negative"; got != want {
		t.Fatalf("error is %q, want %q", got, want)
	}
}
