// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// Refused carries the ceiling that denied this attempt. It reads no counter:
// a refusal has no admission left, and waiting a full window is always safe.
func Refused(limiter *Limiter) error {
	return RefusedBecause(limiter, "the request limit has been reached; try again after the wait")
}

// RefusedBecause keeps an operation's actionable sentence with its pacing data.
func RefusedBecause(limiter *Limiter, detail string) error {
	return &refusalError{ceiling: limiter.Limit(), wait: limiter.RetryAfterSeconds(), detail: detail}
}

// WaitOnly hides which shared guard refused an anonymous request. In particular,
// a link request must never reveal an email address's earlier request history.
func WaitOnly(wait int) error {
	return &refusalError{wait: wait, detail: "requests are refused for now; try again after the wait"}
}

type refusalError struct {
	ceiling, wait int
	detail        string
}

func (e *refusalError) Error() string          { return e.detail }
func (e *refusalError) Unwrap() error          { return apperrors.ErrBudgetExceeded }
func (e *refusalError) RetryAfterSeconds() int { return e.wait }
func (e *refusalError) RateLimit() (int, int)  { return e.ceiling, e.wait }

// SetRefusalHeaders reports a known pacing refusal, including through wrappers.
// Errors about quotas or resources without a timed limiter supply no headers.
func SetRefusalHeaders(header http.Header, err error) {
	var paced interface{ RateLimit() (int, int) }
	if errors.As(err, &paced) {
		ceiling, wait := paced.RateLimit()
		SetHeaders(header, ceiling, wait)
	}
}

// SetHeaders publishes a refusal's limit and a conservative retry window.
// Remaining is the number of attempts admitted now, including a fail-closed
// store outage. Reset is a relative wait, never a replica's local wall clock.
func SetHeaders(header http.Header, ceiling, wait int) {
	if wait < 1 {
		return
	}
	if ceiling > 0 {
		header.Set("RateLimit-Limit", strconv.Itoa(ceiling))
	} else {
		header.Del("RateLimit-Limit")
	}
	header.Set("RateLimit-Remaining", "0")
	header.Set("RateLimit-Reset", strconv.Itoa(wait))
	header.Set("Retry-After", strconv.Itoa(wait))
}
