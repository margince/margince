// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// allowSend is the guard for a unit test that has no row to re-check.
func allowSend() error { return nil }

// loopbackClient is the delivery client pointed at a test receiver, which
// netguard would refuse.
func loopbackClient(tlsConfig *tls.Config) *http.Client {
	return NewDeliveryClient((&net.Dialer{Timeout: deliveryTimeout}).DialContext, tlsConfig)
}

func testDeliverer(t *testing.T, client HTTPDoer) (*Deliverer, string) {
	t.Helper()
	cipher, err := NewCipher(make([]byte, WebhookKeyBytes))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := generateSecret()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := cipher.seal(secret)
	if err != nil {
		t.Fatal(err)
	}
	return NewDeliverer(NewStore(nil, cipher), client,
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, nil, slog.New(slog.NewTextHandler(io.Discard, nil))), sealed
}

func transportTLS(t *testing.T, server *httptest.Server) *tls.Config {
	t.Helper()
	transport, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("the test server's client has no *http.Transport")
	}
	return transport.TLSClientConfig
}

// earlyAnswerListener answers 200 the moment a TLS connection is up, before
// it reads anything, then counts the request bytes that arrive.
func earlyAnswerListener(t *testing.T) (string, *tls.Config, <-chan int64) {
	t.Helper()
	cert := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(cert.Close)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cert.TLS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ln.Close(); err != nil {
			t.Log(err)
		}
	})
	received := make(chan int64, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			received <- -1
			return
		}
		//craft:ignore swallowed-errors a refused attempt may close before this answer lands; the byte count below is the check
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		//craft:ignore swallowed-errors the count is the result; the client's close is what ends the copy
		n, _ := io.Copy(io.Discard, conn)
		received <- n
	}()
	return "https://" + ln.Addr().String() + "/hook", transportTLS(t, cert), received
}

// A refused check closes the attempt's connection before the transport writes
// the request. The receiver gets no byte, not even headers, and its early 200
// does not make the attempt delivered.
func TestARefusedSendCheckWritesNoByteOfTheRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		guard func() error
		want  outcome
	}{
		"withdrawn": {guard: func() error { return errSendWithdrawn }, want: outcome{withdrawn: true}},
		"unchecked": {guard: func() error { return errSendUnchecked }, want: outcome{failure: sendUncheckedReason}},
	} {
		t.Run(name, func(t *testing.T) {
			url, tlsConfig, received := earlyAnswerListener(t)
			d, sealed := testDeliverer(t, loopbackClient(tlsConfig))
			target := attemptTarget{
				deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: url, sealedSecret: sealed,
				eventType: "contact.updated", eventID: ids.NewV7(), payload: []byte(`{"full_name":"A contact"}`),
			}
			if got := d.attempt(context.Background(), target, tc.guard); got != tc.want {
				t.Fatalf("attempt = %+v, want %+v", got, tc.want)
			}
			if n := <-received; n != 0 {
				t.Fatalf("the receiver read %d request bytes from a refused attempt, want 0", n)
			}
		})
	}
}

// A passed check sends the body whole, over HTTP/1.1 even to a receiver that
// offers HTTP/2.
func TestACheckedAttemptSendsItsBodyOverHTTP1(t *testing.T) {
	var gotBody string
	var gotProto int
	receiver := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotBody, gotProto = string(body), r.ProtoMajor
		w.WriteHeader(http.StatusOK)
	}))
	receiver.EnableHTTP2 = true
	receiver.StartTLS()
	defer receiver.Close()
	d, sealed := testDeliverer(t, loopbackClient(transportTLS(t, receiver)))
	payload := `{"full_name":"A contact"}`
	target := attemptTarget{
		deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: receiver.URL, sealedSecret: sealed,
		eventType: "contact.updated", eventID: ids.NewV7(), payload: []byte(payload),
	}
	if got := d.attempt(context.Background(), target, allowSend); got != (outcome{statusCode: http.StatusOK}) {
		t.Fatalf("attempt = %+v, want a 200 delivery", got)
	}
	if gotBody != payload || gotProto != 1 {
		t.Fatalf("receiver got %q over HTTP/%d, want %q over HTTP/1", gotBody, gotProto, payload)
	}
}

// answersWithoutDialing is a transport that never opens a connection, so no
// send check runs on it.
type answersWithoutDialing struct{}

func (answersWithoutDialing) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

// A 2xx counts only when the check passed on the connection that took the
// whole body. A refusal wins over any answer.
func TestA2xxCountsOnlyOverACheckedConnectionThatTookTheBody(t *testing.T) {
	d, sealed := testDeliverer(t, answersWithoutDialing{})
	target := attemptTarget{deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: "https://hooks.example/x", sealedSecret: sealed, payload: []byte(`{}`)}
	if got := d.attempt(context.Background(), target, allowSend); got.failure != uncheckedConnectionReason {
		t.Fatalf("a 2xx from a transport that ran no check = %+v, want %q", got, uncheckedConnectionReason)
	}

	passed := &sendCheck{verify: allowSend}
	if err := passed.run(); err != nil {
		t.Fatal(err)
	}
	partly := &countedBody{}
	partly.read.Store(1)
	if got := checkedOutcome(passed, partly, 2, outcome{statusCode: http.StatusOK}); got.failure != earlyAnswerReason {
		t.Fatalf("a 2xx before the whole body was taken = %+v, want %q", got, earlyAnswerReason)
	}

	refused := &sendCheck{verify: func() error { return errSendWithdrawn }}
	if err := refused.run(); err == nil {
		t.Fatal("a withdrawn check reported no error")
	}
	if got := checkedOutcome(refused, partly, 1, outcome{statusCode: http.StatusOK}); !got.withdrawn {
		t.Fatalf("a refused check with a 2xx = %+v, want withdrawn", got)
	}
}

// Each attempt dials its own connection, so a receiver that keeps the first
// one open cannot carry a second attempt past its check.
func TestEachAttemptIsCheckedOnItsOwnConnection(t *testing.T) {
	var requests atomic.Int64
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	d, sealed := testDeliverer(t, loopbackClient(transportTLS(t, receiver)))
	target := attemptTarget{deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: receiver.URL, sealedSecret: sealed, payload: []byte(`{}`)}
	if got := d.attempt(context.Background(), target, allowSend); got.failure != "" {
		t.Fatalf("the first attempt failed: %s", got.failure)
	}
	if got := d.attempt(context.Background(), target, func() error { return errSendWithdrawn }); !got.withdrawn {
		t.Fatalf("the second attempt = %+v, want withdrawn by its own check", got)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("the receiver read %d requests, want only the first", n)
	}
}
