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

// A snapshot share says nobody had called only where the snapshot recorded
// it: a manager-call setting over every pipeline, with no call amount kept.
func TestASnapshotSaysNobodyCalledOnlyWhereItRecordedThat(t *testing.T) {
	t.Parallel()
	managerCall, likely, kept := string(MeasureManagerCall), string(MeasureLikely), int64(3_000_000)
	for _, tc := range []struct {
		name                   string
		mayAssert, onePipeline bool
		forward, landing       *string
		callMinor              *int64
		wantAmount             *int64
		wantWithheld, wantNone bool
	}{
		{"manager call, every pipeline, no call", true, false, &managerCall, &likely, nil, nil, false, true},
		{"manager call, a call kept", true, false, &managerCall, &managerCall, &kept, &kept, false, false},
		{"the Likely measure", true, false, &likely, &likely, nil, nil, true, false},
		{"one pipeline", true, true, &managerCall, &likely, nil, nil, true, false},
		{"no landing kept", true, false, nil, nil, nil, nil, true, false},
		{"a reader who may not see calls", false, false, &managerCall, &managerCall, &kept, nil, true, false},
	} {
		got, noneStood := FrozenCallSeen(tc.mayAssert, FrozenCall{
			ForwardMeasure: tc.forward, LandingMeasure: tc.landing, OnePipeline: tc.onePipeline, CallMinor: tc.callMinor,
		})
		if got.Withheld != tc.wantWithheld || noneStood != tc.wantNone || (got.Amount == nil) != (tc.wantAmount == nil) {
			t.Errorf("%s: landing call %+v, none stood %v; want withheld %v, none stood %v, amount %v",
				tc.name, got, noneStood, tc.wantWithheld, tc.wantNone, tc.wantAmount)
		}
	}
}

// A managed-teams reading covers several populations, so it has no one call.
// It withholds the call rather than say nobody made one, and reads nothing.
func TestAManagedTeamsReadingWithholdsTheCall(t *testing.T) {
	t.Parallel()
	_, seen, err := CallSeen(t.Context(), nil, nil, true, Period{}, Scope{Kind: ScopeManagedTeams})
	if err != nil || !seen.Withheld || seen.Amount != nil {
		t.Fatalf("a managed-teams reading saw %+v (%v), want the call withheld", seen, err)
	}
}
