package view

import (
	"context"
	"io"
)

// Live renders the live feed: traces in flight and completed traces in one stream.
func Live(ctx context.Context, w io.Writer, page Page) error {
	return live(page).Render(ctx, w)
}
