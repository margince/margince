// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// statusClientClosedRequest is the status a request is recorded under when its
// caller went away before the answer: a 4xx by convention, because the fault is
// not the server's, and never written to anyone since nobody is reading.
const statusClientClosedRequest = 499

// CallerLeft is true when the request's own context was canceled. The error
// need not say so. pgx closes the connection under a canceled read, and the
// next read on it fails with a "conn closed" that names no context error.
// A server deadline ends the context with DeadlineExceeded. A cancel deeper in
// the work while the caller waits leaves the request context live. Neither
// qualifies.
func CallerLeft(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.Canceled)
}

// requestTimedOutCode is a request stopped because it ran past the time the
// server gives one answer.
const requestTimedOutCode = "request_timed_out"

// requestTimedOutRetryAfter is the wait, in seconds, a timed-out request is
// told to take. A slow request is most often one queued behind other writes.
const requestTimedOutRetryAfter = 2

// RanOutOfTime is true when the request's own context passed its deadline. The
// request context carries no deadline but the server's own bound, so no deeper
// timeout qualifies.
func RanOutOfTime(ctx context.Context) bool {
	return errors.Is(ctx.Err(), context.DeadlineExceeded)
}

// timedOutFault answers err when the server's own deadline caused it. A driver
// error after the deadline often names no context error, so an unclassified
// error counts too. A classified one keeps its answer unless it is the deadline.
func timedOutFault(r *http.Request, err error, classified bool) (Fault, bool) {
	if (classified && !errors.Is(err, context.DeadlineExceeded)) || !RanOutOfTime(r.Context()) {
		return Fault{}, false
	}
	slog.WarnContext(r.Context(), "request ran past its time limit",
		"method", r.Method, "path", loggedPath(r), "err", err)
	return Fault{
		Status: http.StatusServiceUnavailable, Code: requestTimedOutCode,
		Detail:            "The request took longer than the server allows and was stopped. Send it again in a moment.",
		RetryAfterSeconds: requestTimedOutRetryAfter,
	}, true
}
