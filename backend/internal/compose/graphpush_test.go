// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Graph notification endpoint's admission, which is the half a database
// test cannot reach: the handshake Microsoft demands before it will create a
// subscription, and the token that must gate it.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// challengeSpec is the Graph spec with the routing half stubbed out: every case
// here stops at or before Handle, and a real one would want a pool.
func challengeSpec(t *testing.T, token string) http.Handler {
	t.Helper()
	spec := graphPushSpec(nil, nil, token, slog.New(slog.DiscardHandler))
	spec.Handle = func(_ context.Context, _ *http.Request, _ []byte) (Disposition, error) {
		t.Error("a request reached Handle that should have stopped at admission")
		return Poison, nil
	}
	return Webhook(spec, slog.New(slog.DiscardHandler))
}

// Microsoft will not create a subscription until the notification URL echoes a
// token it POSTs there. Verbatim, as text/plain — it compares the bytes.
func TestTheGraphHandshakeEchoesMicrosoftsTokenVerbatim(t *testing.T) {
	const token = "operator-token"
	const validation = "Validation: Testing client application reachability for subscription"

	// Percent-encoded on the wire, which is how Microsoft sends it — the token
	// carries spaces and colons. What must be echoed is the DECODED value.
	q := url.Values{"token": {token}, "validationToken": {validation}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/graph?"+q.Encode(), nil)
	challengeSpec(t, token).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handshake → %d, want 200; Microsoft refuses to create the subscription otherwise", rec.Code)
	}
	if body := rec.Body.String(); body != validation {
		t.Errorf("echoed %q, want the token verbatim", body)
	}
	// text/plain, because Microsoft's own check reads it as text — and because
	// the bytes are the caller's, so nothing may re-interpret them as markup.
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("the echo is served without nosniff; a browser may sniff attacker-chosen bytes under our own origin")
	}
}

// The handshake runs AFTER the token check. Answering before it would make this
// endpoint an echo oracle for anybody who found the URL, reflecting
// attacker-chosen bytes under our own origin.
func TestTheGraphHandshakeIsNotAnEchoOracle(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/webhooks/graph?token=wrong&validationToken=reflect-me", nil)
	challengeSpec(t, "operator-token").ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("an unauthenticated handshake → %d, want 401", rec.Code)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("a refused handshake answered %q; it must reflect nothing", body)
	}
}

// A notification is not a handshake: without validationToken the request goes
// to Handle, which is where routing and the response discipline live.
func TestAGraphNotificationIsNotTakenForAHandshake(t *testing.T) {
	const token = "operator-token"
	spec := graphPushSpec(nil, nil, token, slog.New(slog.DiscardHandler))
	reached := false
	spec.Handle = func(_ context.Context, _ *http.Request, _ []byte) (Disposition, error) {
		reached = true
		return Accepted, nil
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/graph?token="+token,
		strings.NewReader(`{"value":[{"clientState":"rep@myco.com"}]}`))
	Webhook(spec, slog.New(slog.DiscardHandler)).ServeHTTP(rec, req)

	if !reached {
		t.Fatal("a notification stopped at the handshake")
	}
	// 202, which is what Microsoft treats as delivered; anything else counts
	// against the endpoint's health and enough of those drop the subscription.
	if rec.Code != http.StatusAccepted {
		t.Errorf("notification → %d, want 202", rec.Code)
	}
}

// The batch envelope, without a database: what the handler makes of a payload
// decides whether Microsoft retries, and the three answers are not
// interchangeable.
func TestAGraphBatchRoutesOneMailboxOnceAndRefusesAnUnroutableOne(t *testing.T) {
	// The SENTINEL as well as the disposition. Poison is the same answer for
	// every refusal here, so a table reading only that cannot tell one cause
	// from another — and the poison log is the whole of what an operator has to
	// go on. An empty batch reported "carries no clientState", which sends the
	// reader looking for an entry there is none of.
	for name, tc := range map[string]struct {
		body  string
		want  Disposition
		bumps int
		cause error
	}{
		"a batch for one mailbox": {
			`{"value":[{"clientState":"rep@myco.com"},{"clientState":"rep@myco.com"}]}`, Accepted, 1, nil,
		},
		"two mailboxes in one delivery": {
			`{"value":[{"clientState":"a@myco.com"},{"clientState":"b@myco.com"}]}`, Accepted, 2, nil,
		},
		// Every subscription this system creates sets clientState, so one
		// without it belongs to somebody else's subscription against this URL:
		// unroutable, and nothing a redelivery would fix.
		"no clientState anywhere": {`{"value":[{}]}`, Poison, 0, errUnroutableDelivery},
		"an empty batch":          {`{"value":[]}`, Poison, 0, errUnroutableDelivery},
		"not JSON at all":         {`{`, Poison, 0, nil},
	} {
		t.Run(name, func(t *testing.T) {
			var bumped []string
			handle := graphBatchHandler(func(_ context.Context, mailboxes []string) error {
				bumped = append(bumped, mailboxes...)
				return nil
			})
			got, err := handle(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), []byte(tc.body))
			if got != tc.want {
				t.Errorf("disposition = %v, want %v", got, tc.want)
			}
			// nil `cause` means the case makes no claim about which error —
			// a malformed body carries the decoder's own, which is not this
			// package's to name.
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Errorf("refused with %v, want %v — the poison log is all an operator reads, "+
					"and a cause that names the wrong thing sends them looking for it", err, tc.cause)
			}
			if len(bumped) != tc.bumps {
				t.Errorf("bumped %v, want %d mailbox(es) — a sync per notification is the same sync started twice", bumped, tc.bumps)
			}
		})
	}
}

// A routing fault is Transient: the delta can always be walked again, so
// redelivery is exactly the recovery path.
func TestAGraphRoutingFaultAsksMicrosoftToRetry(t *testing.T) {
	handle := graphBatchHandler(func(context.Context, []string) error { return errNoClientState })
	got, err := handle(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil),
		[]byte(`{"value":[{"clientState":"rep@myco.com"}]}`))
	if got != Transient || err == nil {
		t.Fatalf("a routing fault answered (%v, %v), want Transient with the cause", got, err)
	}
}

// The work a delivery costs must not scale with what it names: the fleet walk
// opens a transaction per workspace, so a bump PER ADDRESS made a single public
// request cost the batch size times the installation's workspace count — an
// unauthenticated caller's lever on the connection pool. One call, one walk.
func TestAWideDeliveryCostsOneWalkNotOnePerMailbox(t *testing.T) {
	var batch strings.Builder
	batch.WriteString(`{"value":[`)
	const named = 500
	for i := range named {
		if i > 0 {
			batch.WriteString(",")
		}
		fmt.Fprintf(&batch, `{"clientState":"m%d@myco.com"}`, i)
	}
	batch.WriteString(`]}`)

	walks := 0
	var got []string
	handle := graphBatchHandler(func(_ context.Context, mailboxes []string) error {
		walks++
		got = mailboxes
		return nil
	})
	disposition, err := handle(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), []byte(batch.String()))
	if disposition != Accepted || err != nil {
		t.Fatalf("a wide delivery answered (%v, %v), want it accepted — Microsoft batches per notification URL and a large installation's burst genuinely names many mailboxes", disposition, err)
	}
	if walks != 1 {
		t.Errorf("%d fleet walks for one delivery, want exactly 1", walks)
	}
	if len(got) != named {
		t.Errorf("routed %d mailboxes, want all %d — a delivery Microsoft sent must not be silently narrowed", len(got), named)
	}
}

// And the body itself is bounded for what a real batch is, because its LENGTH
// is what buys the fan-out above.
func TestTheGraphBodyBoundIsSizedForANotificationNotAWebhook(t *testing.T) {
	spec := graphPushSpec(nil, nil, "t", slog.New(slog.DiscardHandler))
	if spec.MaxBody > 1<<20 {
		t.Errorf("MaxBody = %d, want a notification-sized bound rather than a generic webhook's megabyte", spec.MaxBody)
	}
}
