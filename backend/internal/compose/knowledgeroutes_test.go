// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Reachability for the knowledge routes, through the MUX rather than past it.
//
// A handler can be written, wired into its seat, forwarded by compose and
// covered by tests, and still be served by nothing: the route only exists if
// the CONTRACT declares the operation. The download was in exactly that state
// — store method, handler, forward, integration tests and a link in the UI, all
// present, and every request to its URL answered 405 because the operation was
// never added to crm.yaml.
//
// Nothing caught it. The generated interface makes a missing HANDLER a compile
// error, which is why that direction feels covered; it says nothing about a
// missing route, and a test that calls the handler directly cannot tell the
// difference. So the assertion here is reachability alone: the method is
// allowed on the path.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

func TestEveryKnowledgeRouteIsServed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		method string
		path   string
	}{
		{"download a document", http.MethodGet, "/v1/knowledge/documents/" + fixedID},
		{"delete a document", http.MethodDelete, "/v1/knowledge/documents/" + fixedID},
		{"list document sets", http.MethodGet, "/v1/knowledge/corpora"},
		{"create a document set", http.MethodPost, "/v1/knowledge/corpora"},
		{"read a document set", http.MethodGet, "/v1/knowledge/corpora/" + fixedID},
		{"edit a document set", http.MethodPatch, "/v1/knowledge/corpora/" + fixedID},
		{"archive a document set", http.MethodDelete, "/v1/knowledge/corpora/" + fixedID},
		{"list a set's documents", http.MethodGet, "/v1/knowledge/corpora/" + fixedID + "/documents"},
		{"upload a document", http.MethodPost, "/v1/knowledge/corpora/" + fixedID + "/documents"},
		{"ask a document set", http.MethodPost, "/v1/knowledge/corpora/" + fixedID + "/ask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			mux := crmcontracts.HandlerFromMuxWithBaseURL(Server{}, chi.NewRouter(), "/v1")
			mux.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			// 405 is the whole point: it means the PATH matched and the METHOD
			// did not, which is what an operation missing from the contract
			// looks like from outside. 404 would mean the path itself is not
			// registered. Anything else — including a panic-recovery 500 from
			// the zero Server reaching a nil seat — proves the route is served,
			// which is all this asserts.
			if rec.Code == http.StatusMethodNotAllowed {
				t.Fatalf("%s %s answered 405: the path is registered but this method is not, so the operation is missing from crm.yaml", tc.method, tc.path)
			}
			if rec.Code == http.StatusNotFound {
				t.Fatalf("%s %s answered 404: no route is registered for it at all", tc.method, tc.path)
			}
		})
	}
}

// fixedID is any well-formed id. Routing does not read it, and a fixed one
// keeps the test free of a clock and a random source.
const fixedID = "01a03dde-7e62-76a1-b6a1-29e4d8c36dc7"
