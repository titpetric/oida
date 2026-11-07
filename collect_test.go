package oida

import (
	"context"
	"errors"
	"testing"

	"github.com/titpetric/oida/model"
)

func TestTracer_Collect(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	// A click of the page whose pageload went first, and one transaction the
	// recorder refuses between them.
	action := pageload()
	action.Name, action.SpanID, action.ParentID = "button event=click", "aaaaaaaaaaaaaaa2", "aaaaaaaaaaaaaaa1"
	receipt, err := tracer.Collect(context.Background(), Collection{Transactions: []Transaction{pageload(), {}, action}})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(receipt.Transactions) != 3 {
		t.Fatalf("the receipt answers %d of 3 transactions", len(receipt.Transactions))
	}
	if receipt.Transactions[1].Error == "" {
		t.Fatal("the refused transaction came back without a reason")
	}
	if receipt.Transactions[0].Error != "" || receipt.Transactions[2].Error != "" {
		t.Fatalf("one bad entry cost the others: %+v", receipt.Transactions)
	}
	if got := receipt.Transactions[2].TraceParent; got != "00-"+exampleTraceID+"-aaaaaaaaaaaaaaa2-01" {
		t.Fatalf("the action reads %q", got)
	}

	if traces := tracer.Traces(); len(traces) != 2 {
		t.Fatalf("retained %d traces, want the two that were valid", len(traces))
	}
}

func TestTracer_CollectRefusesAnEmptyCollection(t *testing.T) {
	tracer, _ := newTestTracer(t, nil)

	if _, err := tracer.Collect(context.Background(), Collection{}); !errors.Is(err, model.ErrInvalidTransaction) {
		t.Fatalf("collect returned %v, want ErrInvalidTransaction", err)
	}
}
