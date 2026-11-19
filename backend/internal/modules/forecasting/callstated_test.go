// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// recordCallRefusal posts a call body and answers the body of its 422. Each
// body below is refused before the store, so the handler needs none.
func recordCallRefusal(t *testing.T, body string) string {
	t.Helper()
	h := Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/v1/forecast/calls", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.RecordForecastCall(rec, req, crmcontracts.RecordForecastCallParams{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("%s answered %d %s, want 422", body, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestACallWithNoAmountIsRefusedRatherThanRecordedAsZero(t *testing.T) {
	for _, body := range []string{
		`{"currency":"EUR"}`,
		`{"amount_minor":null,"currency":"EUR"}`,
	} {
		if got := recordCallRefusal(t, body); !strings.Contains(got, `"field":"amount_minor"`) {
			t.Errorf("%s was refused with %s, want amount_minor named", body, got)
		}
	}
}

// A typed 0 is a call. It passes the amount check and is refused here only for
// the team it does not name.
func TestATypedZeroIsACall(t *testing.T) {
	got := recordCallRefusal(t, `{"amount_minor":0,"currency":"EUR","scope_kind":"team"}`)
	if strings.Contains(got, `"field":"amount_minor"`) || !strings.Contains(got, `"field":"scope_id"`) {
		t.Fatalf("a typed 0 was refused with %s, want only the missing scope_id named", got)
	}
}
