// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package retryafter_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/retryafter"
)

// The instant every date case is measured against, so no case reads a clock.
var now = time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC)

func TestTheDeltaSecondsForm(t *testing.T) {
	if got := retryafter.HeaderAt("120", now); got != 2*time.Minute {
		t.Errorf("HeaderAt(%q) = %v, want 2m", "120", got)
	}
}

// The half four connectors were missing. A provider answering in the date form
// was read as having said nothing, and the caller came back on its own backoff
// — early, on a rate limit, which is how a throttle becomes a ban.
func TestTheHTTPDateForm(t *testing.T) {
	if got := retryafter.HeaderAt("Wed, 21 Oct 2026 07:28:00 GMT", now); got != 28*time.Minute {
		t.Errorf("HeaderAt(<a date 28 minutes out>) = %v, want 28m", got)
	}
}

// A date already past is not a negative wait. It is the provider saying the
// window has closed, which is the same instruction as saying nothing.
func TestADateAlreadyPastAsksForNoWait(t *testing.T) {
	if got := retryafter.HeaderAt("Wed, 21 Oct 2026 06:30:00 GMT", now); got != 0 {
		t.Errorf("HeaderAt(<a date in the past>) = %v, want 0", got)
	}
}

// Zero means retry now, and a negative is malformed. Neither is an interval to
// wait, and a caller that treated zero as one would hammer a provider that has
// just asked it to slow down.
func TestNonPositiveSecondsAskForNoWait(t *testing.T) {
	for _, value := range []string{"0", "-5"} {
		if got := retryafter.HeaderAt(value, now); got != 0 {
			t.Errorf("HeaderAt(%q) = %v, want 0", value, got)
		}
	}
}

func TestAnUnreadableHeaderAsksForNoWait(t *testing.T) {
	for _, value := range []string{"", "   ", "soon", "12 seconds", "1.5"} {
		if got := retryafter.HeaderAt(value, now); got != 0 {
			t.Errorf("HeaderAt(%q) = %v, want 0 — an unreadable header is not an interval", value, got)
		}
	}
}

// Surrounding space is the provider's, not the value's.
func TestSurroundingSpaceIsIgnored(t *testing.T) {
	if got := retryafter.HeaderAt("  30  ", now); got != 30*time.Second {
		t.Errorf("HeaderAt(%q) = %v, want 30s", "  30  ", got)
	}
}

func TestOfReadsTheHeaderOffAResponse(t *testing.T) {
	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "45")
	if got := retryafter.Of(resp); got != 45*time.Second {
		t.Errorf("Of(<a response asking for 45s>) = %v, want 45s", got)
	}
}

// A nil response is what a transport hands back beside an error, and every
// caller reaches for the wait before it knows which it got.
func TestOfSurvivesANilResponse(t *testing.T) {
	if got := retryafter.Of(nil); got != 0 {
		t.Errorf("Of(nil) = %v, want 0", got)
	}
}

func TestOfIsZeroWhenTheProviderSaidNothing(t *testing.T) {
	if got := retryafter.Of(&http.Response{Header: http.Header{}}); got != 0 {
		t.Errorf("Of(<a response with no Retry-After>) = %v, want 0", got)
	}
}
