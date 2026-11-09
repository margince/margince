// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// allowSend is the guard for a unit test that has no row to re-check.
func allowSend() error { return nil }

// A refused guard stops the body at the transport. The receiver reads no byte
// of it, and the attempt reports itself withdrawn rather than failed.
func TestARefusedSendGuardKeepsTheBodyOffTheWire(t *testing.T) {
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
	var bodyBytes, complete atomic.Int64
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		bodyBytes.Add(int64(len(body)))
		if err == nil {
			complete.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	d := NewDeliverer(NewStore(nil, cipher), receiver.Client(),
		func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	target := attemptTarget{
		deliveryID: ids.NewV7(), subID: ids.NewV7(), targetURL: receiver.URL, sealedSecret: sealed,
		eventType: "contact.updated", eventID: ids.NewV7(), payload: []byte(`{"full_name":"Erased Person"}`),
	}

	for name, tc := range map[string]struct {
		guard     func() error
		want      outcome
		wantBytes int64
	}{
		"withdrawn": {guard: func() error { return errSendWithdrawn }, want: outcome{withdrawn: true}},
		"unchecked": {guard: func() error { return errSendUnchecked }, want: outcome{failure: sendUncheckedReason}},
		"allowed":   {guard: allowSend, want: outcome{statusCode: http.StatusOK}, wantBytes: int64(len(target.payload))},
	} {
		t.Run(name, func(t *testing.T) {
			bodyBytes.Store(0)
			complete.Store(0)
			if got := d.attempt(context.Background(), target, tc.guard); got != tc.want {
				t.Fatalf("attempt = %+v, want %+v", got, tc.want)
			}
			if got := bodyBytes.Load(); got != tc.wantBytes {
				t.Fatalf("the receiver read %d body bytes, want %d", got, tc.wantBytes)
			}
			if tc.wantBytes == 0 && complete.Load() != 0 {
				t.Fatal("the receiver read a complete request from a refused attempt")
			}
		})
	}
}
