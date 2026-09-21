// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/testmailbox"
)

// A connectorHandlers whose registry has test_mailbox registered — the
// AllowTestMailbox:true case — but with no vault/DB, so only the
// pre-Connect() gating (registration presence, human-only) is exercised
// here; the full connect-and-persist path is the integration test
// (backend/internal/compose/integration/testmailbox_integration_test.go).
func testMailboxWiredHandlers() connectorHandlers {
	return connectorHandlers{
		registry:      NewCaptureRegistry(nil, nil, CaptureConfig{AllowTestMailbox: true}),
		authority:     liveAuthority{},
		signer:        newStateSigner([]byte(testStateKey)),
		publicBaseURL: "https://app.test",
		apiBaseURL:    "https://api.test",
	}
}

func TestConnectTestMailboxRefusedWhenNotRegistered(t *testing.T) {
	h := wiredHandlers() // AllowTestMailbox not set — test_mailbox absent from this registry
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/test_mailbox/connect", nil).WithContext(humanCtx())

	h.ConnectConnector(rec, req, crmcontracts.CaptureProvider(testmailbox.Name))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (connector_unsupported) when test_mailbox is not registered", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "connector_unsupported") {
		t.Errorf("body should carry connector_unsupported: %s", rec.Body)
	}
}

func TestConnectTestMailboxRefusesAnUnauthenticatedRequest(t *testing.T) {
	h := testMailboxWiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/test_mailbox/connect", nil) // no humanCtx

	h.ConnectConnector(rec, req, crmcontracts.CaptureProvider(testmailbox.Name))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an unauthenticated request", rec.Code)
	}
}
