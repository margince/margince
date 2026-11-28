// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

func TestGraphEndpointValidationIsPlainTextAndNeverProcessesAConversation(t *testing.T) {
	receiver := &countingReceiver{}
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	handler := EventHandlers{graph: receiver, graphPeer: ratelimit.NewWithClock("graph-validation", ratelimit.FailClosed, 10, time.Minute, func() time.Time { return now })}
	token := "<validation & unicode: ✓>"
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/channel-events/teams/graph", strings.NewReader("unused body"))
	handler.ReceiveTeamsGraphNotification(response, request, crmcontracts.ReceiveTeamsGraphNotificationParams{ValidationToken: &token})
	if response.Code != http.StatusOK || response.Body.String() != token || response.Header().Get("Content-Type") != "text/plain; charset=utf-8" || response.Header().Get("X-Content-Type-Options") != "nosniff" || receiver.calls != 0 {
		t.Fatalf("validation changed or processed token: %d %q calls=%d", response.Code, response.Body.String(), receiver.calls)
	}
	token = strings.Repeat("x", 4097)
	response = httptest.NewRecorder()
	handler.ReceiveTeamsGraphNotification(response, request, crmcontracts.ReceiveTeamsGraphNotificationParams{ValidationToken: &token})
	if response.Code != http.StatusBadRequest || receiver.calls != 0 {
		t.Fatalf("oversized validation token accepted: %d", response.Code)
	}
	response = httptest.NewRecorder()
	handler.ReceiveTeamsGraphNotification(response, httptest.NewRequest(http.MethodPost, "/v1/channel-events/teams/graph", strings.NewReader(`{"value":[]}`)), crmcontracts.ReceiveTeamsGraphNotificationParams{})
	if response.Code != http.StatusOK || receiver.calls != 1 {
		t.Fatalf("normal delivery not delegated: %d calls=%d", response.Code, receiver.calls)
	}
}
