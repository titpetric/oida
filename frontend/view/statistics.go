package view

import (
	"context"
	"io"
)

// Statistics renders the rolling window grouped by route.
func Statistics(ctx context.Context, w io.Writer, page Page) error {
	return statistics(page).Render(ctx, w)
}
