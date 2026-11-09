// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"bytes"
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

// allowSend is the hold for a unit test that has no row to re-check.
func allowSend() (func(), error) { return func() {}, nil }

func refuse(err error) sendHold { return func() (func(), error) { return nil, err } }

// recordingHold allows the send and closes released when the hold ends.
func recordingHold() (sendHold, <-chan struct{}) {
	released := make(chan struct{})
	return func() (func(), error) { return func() { close(released) }, nil }, released
}

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
// it reads anything. If reads, it then counts the request bytes that arrive;
// otherwise it never reads and holds the connection open until the test ends.
func earlyAnswerListener(t *testing.T, reads bool) (string, *tls.Config, <-chan int64) {
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
	received, stopped := make(chan int64, 1), make(chan struct{})
	t.Cleanup(func() { close(stopped) })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			received <- -1
			return
		}
		//craft:ignore swallowed-errors a refused attempt may close before this answer lands; the byte count below is the check
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n")
		if !reads {
			<-stopped
			received <- 0
			return
		}
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
		guard sendHold
		want  outcome
	}{
		"withdrawn": {guard: refuse(errSendWithdrawn), want: outcome{withdrawn: true}},
		"unchecked": {guard: refuse(errSendUnchecked), want: outcome{failure: sendUncheckedReason}},
	} {
		t.Run(name, func(t *testing.T) {
			url, tlsConfig, received := earlyAnswerListener(t, true)
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

// A 2xx counts only for a request written whole under the hold. A refusal
// wins over any answer.
func TestA2xxCountsOnlyForARequestWrittenUnderTheHold(t *testing.T) {
	d, sealed := testDeliverer(t, answersWithoutDialing{})
	target := attemptTarget{deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: "https://hooks.example/x", sealedSecret: sealed, payload: []byte(`{}`)}
	if got := d.attempt(context.Background(), target, allowSend); got.failure != unwrittenReason {
		t.Fatalf("a 2xx from a transport that took no hold = %+v, want %q", got, unwrittenReason)
	}
	ok := outcome{statusCode: http.StatusOK}
	for name, tc := range map[string]struct {
		state int32
		want  outcome
	}{
		"held, write not finished": {checkHeld, outcome{statusCode: http.StatusOK, failure: unwrittenReason}},
		"refused":                  {checkRefused, outcome{withdrawn: true}},
		"written":                  {checkWritten, ok},
	} {
		check := &sendCheck{}
		check.state.Store(tc.state)
		if got := checkedOutcome(check, ok); got != tc.want {
			t.Errorf("%s with a 2xx = %+v, want %+v", name, got, tc.want)
		}
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
	if got := d.attempt(context.Background(), target, refuse(errSendWithdrawn)); !got.withdrawn {
		t.Fatalf("the second attempt = %+v, want withdrawn by its own check", got)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("the receiver read %d requests, want only the first", n)
	}
}

// A receiver that answers at once and never reads holds the row only until the
// write limit. Its answer does not count, because the write did not finish.
func TestAReceiverThatNeverReadsHoldsTheRowOnlyUntilTheWriteLimit(t *testing.T) {
	url, tlsConfig, _ := earlyAnswerListener(t, false)
	client := &http.Client{Timeout: 3 * time.Second, Transport: checkedTransport{
		dial: (&net.Dialer{}).DialContext, tlsConfig: tlsConfig, writeLimit: 200 * time.Millisecond,
	}}
	d, sealed := testDeliverer(t, client)
	hold, released := recordingHold()
	target := attemptTarget{
		deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: url, sealedSecret: sealed,
		payload: bytes.Repeat([]byte("x"), 64<<20),
	}
	got := d.attempt(context.Background(), target, hold)
	if got.failure == "" || !strings.Contains(got.failure, "i/o timeout") {
		t.Fatalf("attempt = %q, want the write limit to end it", got.failure)
	}
	select {
	case <-released:
	default:
		t.Fatal("the attempt returned with the row still held")
	}
}

// The hold ends once the request is written, before the response is awaited:
// this receiver answers only after it sees the hold end.
func TestTheHoldEndsBeforeTheResponseIsAwaited(t *testing.T) {
	hold, released := recordingHold()
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		select {
		case <-released:
			w.WriteHeader(http.StatusOK)
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusConflict)
		}
	}))
	defer receiver.Close()
	d, sealed := testDeliverer(t, loopbackClient(transportTLS(t, receiver)))
	target := attemptTarget{deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: receiver.URL, sealedSecret: sealed, payload: []byte(`{}`)}
	if got := d.attempt(context.Background(), target, hold); got != (outcome{statusCode: http.StatusOK}) {
		t.Fatalf("attempt = %+v, want 200: the receiver answers only once the hold has ended", got)
	}
}

// wroteAfterRelease records a request write that reached the wire after the
// hold ended. Writes once the response is being read are the TLS close, not
// the request.
type wroteAfterRelease struct {
	net.Conn
	held, released, reading *atomic.Bool
	late                    *atomic.Int64
}

func (c wroteAfterRelease) Write(p []byte) (int, error) {
	if c.held.Load() && c.released.Load() && !c.reading.Load() {
		c.late.Add(1)
	}
	return c.Conn.Write(p)
}

func (c wroteAfterRelease) Read(p []byte) (int, error) {
	if c.held.Load() {
		c.reading.Store(true)
	}
	return c.Conn.Read(p)
}

// The hold covers every write of the request: none reaches the wire after
// the hold has ended.
func TestTheHoldCoversTheWholeWrite(t *testing.T) {
	receiver := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	var held, released, reading atomic.Bool
	var late atomic.Int64
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		return wroteAfterRelease{Conn: conn, held: &held, released: &released, reading: &reading, late: &late}, nil
	}
	hold := func() (func(), error) {
		held.Store(true)
		return func() { released.Store(true) }, nil
	}
	d, sealed := testDeliverer(t, NewDeliveryClient(dial, transportTLS(t, receiver)))
	target := attemptTarget{
		deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: receiver.URL, sealedSecret: sealed,
		payload: bytes.Repeat([]byte("x"), 256<<10),
	}
	if got := d.attempt(context.Background(), target, hold); got != (outcome{statusCode: http.StatusOK}) {
		t.Fatalf("attempt = %+v, want a 200 delivery", got)
	}
	if !released.Load() {
		t.Fatal("the hold never ended")
	}
	if n := late.Load(); n != 0 {
		t.Fatalf("%d write(s) of the request reached the wire after the hold ended", n)
	}
}
