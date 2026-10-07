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

// callerLeft is true when err is the request's own cancellation: the context
// error AND a request context that is itself canceled. A deadline the server
// set is its own failure to finish, and a canceled context deeper in the work
// while the caller still waits is a fault, so neither qualifies.
func callerLeft(r *http.Request, err error) bool {
	return errors.Is(err, context.Canceled) && errors.Is(r.Context().Err(), context.Canceled)
}
