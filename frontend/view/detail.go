package view

import (
	"context"
	"io"
)

// Detail renders the trace detail: the timeline, the spans, the logs and the facts beside them.
func Detail(ctx context.Context, w io.Writer, page Page) error {
	return detail(page).Render(ctx, w)
}
