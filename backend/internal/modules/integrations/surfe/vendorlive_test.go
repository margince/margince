// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build vendorlive

package surfe

// The live half of vendorwire_test.go: the same credit read, against Surfe
// itself. The scheduled lane runs it daily with a low-limit key and files an
// issue from the one VENDOR_LIVE line it prints. Run it by hand with
// `MARGINCE_SURFE_LIVE_KEY=… make vendor-live`.
//
// The output carries a status code and field names only. The response body
// and the key never reach a log line, because the run log is public.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// liveAnswer keeps what the real transport returned, so the test can compare
// the answer's field names with the recording. The adapter's own decode
// cannot: a dropped field reads as a zero balance.
type liveAnswer struct {
	inner  HTTPDoer
	status int
	body   []byte
}

func (l *liveAnswer) Do(req *http.Request) (*http.Response, error) {
	resp, err := l.inner.Do(req)
	if err != nil {
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if closeErr := resp.Body.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, readErr
	}
	l.status, l.body = resp.StatusCode, raw
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}

func TestSurfeLiveCreditRead(t *testing.T) {
	key := liveKey(t)
	readAt := clocktest.Now(t)
	adapter := New(func() time.Time { return readAt })
	answer := &liveAnswer{inner: adapter.client}
	adapter.WithHTTPDoer(answer)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	credits, err := adapter.Credits(ctx, provider.Credential(key))
	switch {
	// The status decides too: a refusal whose body is not JSON fails the decode before the adapter reads the status.
	case errors.Is(err, ErrCredentialRefused), answer.status == http.StatusUnauthorized, answer.status == http.StatusForbidden:
		verdict(t, "refused", fmt.Sprintf("status=%d", answer.status))
	case err != nil && answer.status == 0:
		// Only the class is printed: the run log is public, and a transport error is free text.
		verdict(t, "drift", "status=unreachable")
	case err != nil:
		verdict(t, "drift", fmt.Sprintf("status=%d", answer.status))
	}
	if missing := missingCreditFields(t, answer.body); len(missing) > 0 {
		verdict(t, "drift", "missing="+strings.Join(missing, ","))
	}
	for pool, balance := range credits.Balances {
		if balance < 0 {
			verdict(t, "drift", "negative="+string(pool))
		}
	}
	fmt.Println("VENDOR_LIVE surfe credits ok")
}

func liveKey(t *testing.T) string {
	t.Helper()
	key := strings.TrimSpace(os.Getenv("MARGINCE_SURFE_LIVE_KEY"))
	if key == "" {
		// Fatal, not Skip: this tag is asked for by hand, and a skip would read as a pass.
		t.Fatal("MARGINCE_SURFE_LIVE_KEY is unset: set it to a Surfe API key to run the live credit read")
	}
	return key
}

// missingCreditFields names every field the recorded credits answer carries,
// or the adapter decodes, that the live answer lacks. A body that is not a
// JSON object lacks all of them, so a decode error is that answer.
func missingCreditFields(t *testing.T, body []byte) []string {
	t.Helper()
	want := map[string]bool{}
	var recorded map[string]json.RawMessage
	if err := json.Unmarshal(recordedVendorWire(t).Credits.Response, &recorded); err != nil {
		t.Fatalf("the recorded credits answer is not a JSON object: %v", err)
	}
	for name := range recorded {
		want[name] = true
	}
	for field := range reflect.TypeFor[creditsResult]().Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		want[name] = true
	}
	var live map[string]json.RawMessage
	if err := json.Unmarshal(body, &live); err != nil {
		live = map[string]json.RawMessage{}
	}
	var missing []string
	for name := range want {
		if _, ok := live[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

// verdict prints the one line the scheduled lane parses, then fails.
func verdict(t *testing.T, outcome, detail string) {
	t.Helper()
	fmt.Printf("VENDOR_LIVE surfe credits %s %s\n", outcome, detail)
	t.FailNow()
}
