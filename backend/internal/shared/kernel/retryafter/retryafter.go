// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package retryafter is the one reading of the Retry-After header.
//
// Held by: TestTheRetryAfterReadingIsSpelledOnce (backend/oneretryafter_test.go)
//
// It was spelled six times: four byte-identical copies under capture (Gmail,
// the Google connector base, Graph, the OAuth flow), the geocoder's own, and
// Telegram's, which reads its envelope first and the header only as a fallback.
// Only the geocoder's handled the whole header.
//
// RFC 9110 §10.2.3 gives Retry-After TWO forms — delta-seconds, and an
// HTTP-date. The four capture copies parsed only the first, so a provider
// answering `Retry-After: Wed, 21 Oct 2026 07:28:00 GMT` was read as having
// said nothing at all, and the connector fell back to its own backoff. That is
// not a formatting detail: the whole point of honouring the header is to come
// back when the provider said to rather than when we guessed, and coming back
// early on a rate limit is how a throttle becomes a ban.
//
// Stdlib-only and in kernel because the callers are four capture connectors, a
// platform package and a module — and a module never imports a sibling
// (ADR-0054).
package retryafter

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Of reads how long a response says to wait. Zero means it did not say — the
// header was absent, unparseable, or named an instant already past — and the
// caller falls back to its own backoff.
func Of(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	return Header(resp.Header.Get("Retry-After"))
}

// Header is Of, for a caller that already holds the value.
func Header(value string) time.Duration {
	return HeaderAt(value, time.Now())
}

// HeaderAt is Header measured from a given instant — the whole rule, with the
// one reading of the clock lifted out so a test can state the answer exactly
// rather than assert a range around a real one.
//
// Both RFC 9110 §10.2.3 forms, in the order the spec lists them: delta-seconds
// first, because a bare integer parses cheaply and the common case stays exact.
func HeaderAt(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			// "0" means retry now, and a negative is malformed. Both are
			// "nothing to wait for" rather than "wait none" — a caller that
			// treated zero as an instruction would hammer a provider that
			// just told it to slow down.
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	at, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	// Measured against OUR clock, not the response's Date header. A clock
	// skewed against the provider's would make the wait wrong in whichever
	// direction the skew runs, and this is the clock the sleep is actually
	// measured against — so a wait computed from it lands where it was meant
	// to, skew or no skew.
	if wait := at.Sub(now); wait > 0 {
		return wait
	}
	return 0
}
