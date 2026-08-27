// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type row struct {
	Name string `json:"name"`
}

type listEnvelope struct {
	Data []row  `json:"data"`
	Page string `json:"page"`
}

// A list with no rows goes out as `[]`, never `null`.
//
// The contract says every list envelope is `required: [data, page]` with `data`
// of `type: array`, and `null` is not an array. Go makes the violation the easy
// mistake: a nil slice IS the idiomatic empty list, and the only place it
// behaves differently is encoding/json.
//
// Written as a VALUE, because that is how every handler in this tree calls
// WriteJSON — `crmcontracts.PersonListResponse{Data: people, …}`. A reflection
// fix that only reached a pointer would pass its own unit test and do nothing
// in production, which is the shape this case exists to refuse.
func TestAnEmptyListIsAnArrayNotNull(t *testing.T) {
	for name, body := range map[string]any{
		"a value envelope":   listEnvelope{Page: "p"},
		"a pointer envelope": &listEnvelope{Page: "p"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteJSON(rec, http.StatusOK, body)
			got := strings.TrimSpace(rec.Body.String())
			if strings.Contains(got, `"data":null`) {
				t.Errorf("wrote %s — a client reading this per the contract calls .map on null "+
					"and takes down the screen rendering it", got)
			}
			if !strings.Contains(got, `"data":[]`) {
				t.Errorf("wrote %s, want an empty array for data", got)
			}
		})
	}
}

// And a list that HAS rows is untouched — the normalisation must not be a
// filter that quietly empties a page.
func TestAPopulatedListIsUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, listEnvelope{Data: []row{{Name: "a"}, {Name: "b"}}, Page: "p"})
	var out listEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding what was written: %v", err)
	}
	if len(out.Data) != 2 || out.Data[0].Name != "a" {
		t.Errorf("wrote %v, want both rows in order", out.Data)
	}
}

// A body with no Data field at all — a single record, a problem document — is
// left exactly as it was.
func TestABodyWithNoListIsLeftAlone(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, row{Name: "solo"})
	if got := strings.TrimSpace(rec.Body.String()); got != `{"name":"solo"}` {
		t.Errorf("wrote %s, want the record untouched", got)
	}
}
