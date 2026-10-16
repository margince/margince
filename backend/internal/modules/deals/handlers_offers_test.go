// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The REST door answers the offer request rules it shares with the record
// seam as a 422, before any store call. The handlers here hold no store.

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// brokenBody fails every read, as a connection dropped mid-upload does.
type brokenBody struct{}

func (brokenBody) Read([]byte) (int, error) { return 0, errors.New("connection reset") }

func offerRequest(body io.Reader) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/offers", body)
	r.Header.Set("Content-Type", "application/json")
	return r
}

func TestTheOfferRoutesRefuseASmuggledTotalAsUnprocessable(t *testing.T) {
	h, id, line := Handlers{}, openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7())
	for name, tc := range map[string]struct {
		call func(http.ResponseWriter, *http.Request)
		body string
	}{
		"create offer": {
			func(w http.ResponseWriter, r *http.Request) {
				h.CreateOffer(w, r, id, crmcontracts.CreateOfferParams{})
			},
			`{"currency":"EUR","source":"manual","total_minor":1}`,
		},
		"update offer": {
			func(w http.ResponseWriter, r *http.Request) {
				h.UpdateOffer(w, r, id, crmcontracts.UpdateOfferParams{})
			},
			`{"net_minor":1}`,
		},
		"add line": {
			func(w http.ResponseWriter, r *http.Request) { h.AddOfferLineItem(w, r, id) },
			`{"quantity":1,"line_total":1}`,
		},
		"update line": {
			func(w http.ResponseWriter, r *http.Request) { h.UpdateOfferLineItem(w, r, id, line) },
			`{"line_net_minor":1}`,
		},
	} {
		w := httptest.NewRecorder()
		tc.call(w, offerRequest(strings.NewReader(tc.body)))
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "totals_derived") {
			t.Errorf("%s answered %d %s, want 422 totals_derived", name, w.Code, w.Body.String())
		}
	}
}

func TestTheOfferRoutesRefuseABodyTheyCannotRead(t *testing.T) {
	h, id, line := Handlers{}, openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7())
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"create offer": func(w http.ResponseWriter, r *http.Request) {
			h.CreateOffer(w, r, id, crmcontracts.CreateOfferParams{})
		},
		"add line":    func(w http.ResponseWriter, r *http.Request) { h.AddOfferLineItem(w, r, id) },
		"update line": func(w http.ResponseWriter, r *http.Request) { h.UpdateOfferLineItem(w, r, id, line) },
	} {
		w := httptest.NewRecorder()
		call(w, offerRequest(brokenBody{}))
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "unreadable") {
			t.Errorf("%s answered %d %s, want 422 unreadable", name, w.Code, w.Body.String())
		}
	}
}
