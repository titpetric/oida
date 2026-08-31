package view

import (
	"context"
	"io"
)

// Login renders the sign in screen.
func Login(ctx context.Context, w io.Writer, page Page) error {
	return login(page).Render(ctx, w)
}
