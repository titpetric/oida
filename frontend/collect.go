package frontend

import (
	"encoding/json"
	"net/http"

	"github.com/titpetric/oida/model"
)

// serveCollect records the transactions a client posted and answers with the
// trace context of each, which the page names as the parent of the calls it
// makes next.
//
// It is the one route of the front end that writes, so it answers 404 until
// Options.Collect turns it on, takes POST alone, and reads at most
// model.CollectLimit of a body. A beacon sends text/plain, so the content
// type is not checked.
func (h *handler) serveCollect(w http.ResponseWriter, r *http.Request) {
	if !h.opts.Collect {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "post a collection", http.StatusMethodNotAllowed)
		return
	}

	var collection model.Collection
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, model.CollectLimit))
	if err := decoder.Decode(&collection); err != nil {
		http.Error(w, "the collection did not decode", http.StatusBadRequest)
		return
	}
	for i := range collection.Transactions {
		attribute(&collection.Transactions[i], r)
	}

	receipt, err := model.Collect(r.Context(), h.tracer(), collection)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, receipt)
}

// attribute fills the request metadata the recorder knows better than the
// client: the user agent it posted with, and the service it posted to.
func attribute(tx *model.Transaction, r *http.Request) {
	if tx.HTTP == nil {
		return
	}
	tx.HTTP.UserAgent = r.UserAgent()
	tx.HTTP.RemoteAddress = ""
}
