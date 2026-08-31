package view

import (
	"context"
	"io"
)

// Hosts renders the landing page, one row per host the service answered for.
func Hosts(ctx context.Context, w io.Writer, page Page) error {
	return hosts(page).Render(ctx, w)
}
