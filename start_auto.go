package oida

import (
	"context"

	"github.com/titpetric/oida/internal"
)

// StartAuto is Start with the span name read from a symbol. Pass a function or
// a value and the package, type and function names are joined with a dot, which
// gives names like billing.UserStore.GetUsers without spelling them out.
//
//	ctx, span := oida.StartAuto(ctx, s.GetUsers)
//	defer span.End()
//
// The name comes from reflection and the runtime symbol table, so it does not
// survive a stripped binary and reads oddly for anonymous functions. Use Start
// where either matters, or where the call is hot enough for the reflection to
// show up.
func StartAuto(ctx context.Context, symbol any, kind ...Kind) (context.Context, *Span) {
	return Start(ctx, internal.SymbolName(symbol), kind...)
}
