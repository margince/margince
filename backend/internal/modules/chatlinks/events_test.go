// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/ratelimit/ratelimittest"
)

type countingReceiver struct{ calls int }

func (c *countingReceiver) Receive(context.Context, Delivery) (crmcontracts.DispactChannelEventReceipt, error) {
	c.calls++
	return crmcontracts.DispactChannelEventReceipt{}, nil
}

// With the shared count out, an anonymous notice is refused before the
// receiver opens the vault or reads a row.
func TestANoticeIsRefusedWhileTheLimiterCannotCount(t *testing.T) {
	receiver := &countingReceiver{}
	h := eventHandlersOn(receiver, ratelimittest.Down().New)
	w := httptest.NewRecorder()
	h.ReceiveDispactChannelEvent(w, httptest.NewRequest(http.MethodPost, "/v1/channel-events/dispact", strings.NewReader(`{}`)))
	if w.Code != http.StatusTooManyRequests || receiver.calls != 0 {
		t.Errorf("with the limiter's store down: %d and %d receive(s); want 429 and none", w.Code, receiver.calls)
	}
}
