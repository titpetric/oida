package internal

import (
	"fmt"

	"github.com/titpetric/oida/model"
)

// InvalidOption formats a field level configuration error, wrapping
// model.ErrInvalidOptions so a caller can match on it.
func InvalidOption(field string, reason string) error {
	return fmt.Errorf("%w: %s %s", model.ErrInvalidOptions, field, reason)
}
