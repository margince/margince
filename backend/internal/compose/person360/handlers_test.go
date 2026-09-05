// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The transport's overlay gate, held on its own.
//
// It is the reason the moment ladder never has to know about overlay mode:
// the ladder mints "log an interaction" as available from the page alone, and
// POST /activities is refused for every mirrored workspace, so the card would
// carry a dead verb if this read answered there at all. The nil service in
// each case is the proof that the gate runs FIRST — a handler that reached the
// database before deciding would panic instead of refusing.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func overlayModeReturning(overlay bool, err error) OverlayMode {
	return func(context.Context) (bool, error) { return overlay, err }
}

func TestGetPerson360RefusesAnOverlayWorkspace(t *testing.T) {
	h := NewHandlers(nil, overlayModeReturning(true, nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/people/x/360", nil)

	h.GetPerson360(rec, req, crmcontracts.Id(ids.NewV7()), crmcontracts.GetPerson360Params{})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding the problem body: %v", err)
	}
	if problem.Code != "validation_error" {
		t.Errorf("problem code = %q, want validation_error", problem.Code)
	}
	errs := problem.Details.Errors
	if len(errs) != 1 || errs[0].Code != "unsupported_in_overlay_mode" {
		t.Errorf("problem details.errors = %+v, want one unsupported_in_overlay_mode", errs)
	}
}

// A mode that cannot be resolved refuses too. Degrading to native here would
// serve the whole page — moment verbs included — off native tables a mirrored
// workspace does not fill.
func TestGetPerson360RefusesWhenTheModeCannotBeResolved(t *testing.T) {
	h := NewHandlers(nil, overlayModeReturning(false, errors.New("mode lookup failed")))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/people/x/360", nil)

	h.GetPerson360(rec, req, crmcontracts.Id(ids.NewV7()), crmcontracts.GetPerson360Params{})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 — an unresolved mode is not a native one", rec.Code)
	}
}
