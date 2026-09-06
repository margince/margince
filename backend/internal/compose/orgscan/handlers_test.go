// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package orgscan

// The surface refuses before it reads when it cannot say which system of
// record the workspace reads from, or when that record is the incumbent's:
// the scan is read from native rows, and there are none to read in overlay.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func serve(t *testing.T, h Handlers, method string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, "/v1/organizations/x/scan", nil)
	id := openapi_types.UUID(ids.NewV7())
	if method == http.MethodPost {
		h.EnsureOrganizationScan(rec, req, id)
	} else {
		h.GetOrganizationScan(rec, req, id)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the refusal is not a problem document: %v: %s", err, rec.Body.String())
	}
	return rec.Code, body
}

func TestAnUnwiredModeCheckIsOurFaultNotTheCallers(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		status, body := serve(t, NewHandlers(nil, nil), method)
		if status != http.StatusInternalServerError || body["code"] != "internal" {
			t.Errorf("%s with no mode check = %d %v, want our own fault", method, status, body)
		}
	}
}

func TestOverlayModeIsToldTheScanHasNothingToRead(t *testing.T) {
	overlay := func(context.Context) (bool, error) { return true, nil }
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		status, body := serve(t, NewHandlers(nil, overlay), method)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("%s in overlay = %d %v, want a validation refusal naming the mode", method, status, body)
		}
	}
}

func TestAModeCheckThatFailsIsCarriedNotGuessed(t *testing.T) {
	broken := func(context.Context) (bool, error) { return false, errors.New("the vault is unreachable") }
	status, _ := serve(t, NewHandlers(nil, broken), http.MethodGet)
	if status != http.StatusInternalServerError {
		t.Errorf("a failing mode check = %d, want the failure carried rather than a confident reading", status)
	}
}
