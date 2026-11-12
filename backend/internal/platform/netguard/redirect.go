// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package netguard

import "net/http"

// RefuseRedirect is an http.Client CheckRedirect that never follows one. The
// 3xx comes back as the response, so the caller treats it as the failure it is.
// A followed redirect would dial a host nobody named and carry the request's
// credential or body there.
func RefuseRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// NoRedirect is a copy of client that refuses redirects. A copy, so the
// caller's shared client keeps following them where that is wanted.
func NoRedirect(client *http.Client) *http.Client {
	strict := *client
	strict.CheckRedirect = RefuseRedirect
	return &strict
}
