package view

import (
	"context"
	"io"
)

// LiveSection renders the feed alone, which is what the event stream pushes
// on every change: the page around it is already in the browser.
func LiveSection(ctx context.Context, w io.Writer, page Page) error {
	return liveSection(page).Render(ctx, w)
}
