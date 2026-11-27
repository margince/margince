// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/ratelimit/ratelimittest"
)

type recordingWhatsAppReceiver struct {
	challengeErr, receiveErr error
	query                    url.Values
	body                     []byte
	signature                string
	calls                    int
}

func (r *recordingWhatsAppReceiver) Challenge(_ context.Context, query url.Values) (string, error) {
	r.calls++
	r.query = query
	return query.Get("hub.challenge"), r.challengeErr
}

func (r *recordingWhatsAppReceiver) Receive(_ context.Context, body []byte, signature string) (crmcontracts.WhatsAppWebhookReceipt, error) {
	r.calls++
	r.body, r.signature = body, signature
	return crmcontracts.WhatsAppWebhookReceipt{Stored: 2, Skipped: 1}, r.receiveErr
}

func serveWhatsApp(h WhatsAppEventHandlers, method, target, body, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if signature != "" {
		req.Header.Set("X-Hub-Signature-256", signature)
	}
	rec := httptest.NewRecorder()
	if method == http.MethodGet {
		h.VerifyWhatsAppWebhook(rec, req, crmcontracts.VerifyWhatsAppWebhookParams{})
	} else {
		h.ReceiveWhatsAppWebhook(rec, req)
	}
	return rec
}

func TestTheChallengeIsEchoedAsPlainText(t *testing.T) {
	receiver := &recordingWhatsAppReceiver{}
	h := NewWhatsAppEventHandlers(receiver)
	rec := serveWhatsApp(h, http.MethodGet, "/v1/channel-events/whatsapp?hub.mode=subscribe&hub.verify_token=t&hub.challenge=1158201444", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "1158201444" ||
		!strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("answer = %d %q (%s), want 200 with the bare challenge as nosniff text/plain", rec.Code, rec.Body.String(), rec.Header().Get("Content-Type"))
	}
	if receiver.query.Get("hub.verify_token") != "t" {
		t.Errorf("the receiver saw query %v, want the verify token passed through", receiver.query)
	}
}

func TestARejectedChallengeIs403AndNamesNoReason(t *testing.T) {
	h := NewWhatsAppEventHandlers(&recordingWhatsAppReceiver{challengeErr: ErrWhatsAppEventRejected})
	rec := serveWhatsApp(h, http.MethodGet, "/x?hub.challenge=c", "", "")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "channel_event_rejected") {
		t.Errorf("a rejected challenge answered %d %s, want 403 channel_event_rejected", rec.Code, rec.Body.String())
	}
}

func TestADeliveryReachesTheReceiverWithItsSignature(t *testing.T) {
	receiver := &recordingWhatsAppReceiver{}
	h := NewWhatsAppEventHandlers(receiver)
	rec := serveWhatsApp(h, http.MethodPost, "/v1/channel-events/whatsapp", `{"a":1}`, "sha256=abc")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stored":2`) {
		t.Errorf("answer = %d %s, want 200 with the receipt", rec.Code, rec.Body.String())
	}
	if string(receiver.body) != `{"a":1}` || receiver.signature != "sha256=abc" {
		t.Errorf("receiver saw %q signed %q, want the raw body and the header as sent", receiver.body, receiver.signature)
	}
}

func TestADeliveryRefusalsMapToTheirStatus(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"rejected":    {ErrWhatsAppEventRejected, http.StatusForbidden},
		"unavailable": {ErrWhatsAppEventsUnavailable, http.StatusServiceUnavailable},
	}
	for name, tc := range cases {
		h := NewWhatsAppEventHandlers(&recordingWhatsAppReceiver{receiveErr: tc.err})
		if rec := serveWhatsApp(h, http.MethodPost, "/x", `{}`, "sha256=abc"); rec.Code != tc.want {
			t.Errorf("%s: answered %d, want %d", name, rec.Code, tc.want)
		}
	}
}

func TestABodyOverTheLimitIsRefusedBeforeTheReceiver(t *testing.T) {
	receiver := &recordingWhatsAppReceiver{}
	h := NewWhatsAppEventHandlers(receiver)
	rec := serveWhatsApp(h, http.MethodPost, "/x", strings.Repeat("x", whatsapp.MaxWebhookBodyBytes+1), "sha256=abc")
	if rec.Code != http.StatusRequestEntityTooLarge || receiver.calls != 0 {
		t.Errorf("answered %d with %d receiver call(s), want 413 and none", rec.Code, receiver.calls)
	}
}

func TestAWhatsAppCallIsRefusedWhileTheLimiterCannotCount(t *testing.T) {
	receiver := &recordingWhatsAppReceiver{}
	h := whatsAppEventHandlersOn(receiver, ratelimittest.Down().New)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if rec := serveWhatsApp(h, method, "/x", `{}`, "sha256=abc"); rec.Code != http.StatusTooManyRequests {
			t.Errorf("%s with the limiter's store down answered %d, want 429", method, rec.Code)
		}
	}
	if receiver.calls != 0 {
		t.Errorf("the receiver was called %d time(s) while the limiter was down", receiver.calls)
	}
}

func TestARoleWithoutAReceiverAnswers503(t *testing.T) {
	var h WhatsAppEventHandlers
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		if rec := serveWhatsApp(h, method, "/x", `{}`, ""); rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s answered %d, want 503", method, rec.Code)
		}
	}
}
