// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Whether a call got as far as the network. A schema downgrade describes what
// was SENT, so a call refused locally — an attachment the wire cannot carry, a
// payload that would not serialise, a request that would not build — was sent
// under nothing, and its failed row must not claim a downgrade.

import (
	"context"
	"net/http"
)

// httpAttempt records whether one call handed its request to the HTTP client.
// It rides the call's context because the request is built and sent several
// frames below Complete, where the downgrade is reported, and those frames
// return errors that do not say which side of the send they came from.
type httpAttempt struct{ began bool }

type httpAttemptKey struct{}

// trackHTTPAttempt starts recording for one call. The returned context is the
// one the call's request must be sent under.
func trackHTTPAttempt(ctx context.Context) (context.Context, *httpAttempt) {
	attempt := &httpAttempt{}
	return context.WithValue(ctx, httpAttemptKey{}, attempt), attempt
}

// sendModelRequest hands a request to the network, marking the attempt its
// context carries as begun. Every adapter that reports a schema downgrade sends
// through it, so "began" is decided at the send itself and not at whichever
// failure sites someone remembered to mark.
func sendModelRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	if attempt, tracked := req.Context().Value(httpAttemptKey{}).(*httpAttempt); tracked {
		attempt.began = true
	}
	return client.Do(req)
}
