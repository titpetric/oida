package internal

import (
	"errors"
	"strings"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestInvalidOption(t *testing.T) {
	err := InvalidOption("OIDA_SAMPLE_RATE", "must be a number")

	if !errors.Is(err, model.ErrInvalidOptions) {
		t.Fatalf("err is %v, want it to wrap ErrInvalidOptions", err)
	}
	if got := err.Error(); !strings.Contains(got, "OIDA_SAMPLE_RATE") || !strings.Contains(got, "must be a number") {
		t.Errorf("err reads %q, want the field and the reason", got)
	}
}
