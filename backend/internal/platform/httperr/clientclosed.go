// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"context"
	"errors"
	"net/http"
)

// statusClientClosedRequest is the status a request is recorded under when its
// caller went away before the answer: a 4xx by convention, because the fault is
// not the server's, and never written to anyone since nobody is reading.
const statusClientClosedRequest = 499

// callerLeft is true when the request's own context was canceled. The error
// need not say so. pgx closes the connection under a canceled read, and the
// next read on it fails with a "conn closed" that names no context error.
// A server deadline ends the context with DeadlineExceeded. A cancel deeper in
// the work while the caller waits leaves the request context live. Neither
// qualifies.
func callerLeft(r *http.Request) bool {
	return errors.Is(r.Context().Err(), context.Canceled)
}
