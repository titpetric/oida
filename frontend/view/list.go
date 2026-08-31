package view

import (
	"context"
	"io"
)

// List renders the retained traces, filtered and sorted the way the page asks.
func List(ctx context.Context, w io.Writer, page Page) error {
	return list(page).Render(ctx, w)
}
