// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

func putHistoryBaseline(h connectorHandlers, provider, body string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/connectors/"+provider+"/history-baseline", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.SetConnectorHistoryBaseline(rec, asSeat(req), crmcontracts.CaptureProvider(provider))
	return rec.Code
}

func TestACalendarConnectionTakesNoHistoryBaseline(t *testing.T) {
	h := connectorHandlers{registry: capture.NewRegistry(nil, nil, deadAuthority{}, nil)}
	if got := putHistoryBaseline(h, "gcal", `{"date":"2026-01-01","timezone":"Europe/Berlin"}`); got != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 for a calendar connection", got)
	}
}

func TestAHistoryBaselineOnAMailboxTheCallerHasNotConnectedReadsAsAbsent(t *testing.T) {
	h := connectorHandlers{registry: capture.NewRegistry(nil, nil, deadAuthority{}, nil)}
	if got := putHistoryBaseline(h, "gmail", `{"date":null}`); got != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

func TestAHistoryBaselineWithNoDateKeyIsRefusedRatherThanCleared(t *testing.T) {
	h := connectorHandlers{registry: capture.NewRegistry(nil, nil, deadAuthority{}, nil)}
	if got := putHistoryBaseline(h, "gmail", `{"timezone":"Europe/Berlin"}`); got != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422: only an explicit null clears", got)
	}
}

func TestAHistoryBaselineWithNoRegistryComposedIsUnwired(t *testing.T) {
	var h connectorHandlers
	if got := putHistoryBaseline(h, "gmail", `{"date":null}`); got != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", got)
	}
}
