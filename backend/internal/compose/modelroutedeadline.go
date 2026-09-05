// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The routes whose handler calls a model and waits for the answer, matched by
// the suffix their contract path ends in. A model call runs for tens of
// seconds — measured on a real installation, a reply draft takes 13 to 45 —
// and the server's WriteTimeout is 30s for every other endpoint's protection.
//
// Suffix rather than a full path because each of these is mounted under an
// id-bearing prefix (/activities/{id}/draft-email, /organizations/{id}/dossier),
// and a list of formatted paths would be a second copy of the router that
// drifts the day a route moves.
var modelRouteSuffixes = []string{
	"/draft-email",
	"/dossier",
	"/growth-fit",
	"/meeting-brief",
	"/ask",
	"/intro-note-draft",
	"/intro-request-draft",
	"/role-proposals",
}

// extendDeadlineForModelRoutes gives a handler that calls a model long enough
// to answer, on THAT route only.
//
// The server-wide WriteTimeout stays short on purpose: it bounds every
// endpoint, and a long global deadline lets one slow-reading client hold a
// connection and its handler goroutine for as long as it likes. Raising it was
// the first fix attempted here and it weakens the whole surface to help three
// routes — the same trade the MCP handler already refused
// (modules/agents/httpmcp.go), which extends its own route and says why.
//
// Before this, a draft that took longer than 30s had its connection cut
// mid-response. The reader saw a 502 from whatever proxy sits in front: a
// deadline nobody meant to set, reported as a broken server.
func extendDeadlineForModelRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !callsAModel(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		// A failure here means the handler chain lost Unwrap(): fail loudly
		// rather than serve a response that dies mid-write, which is the very
		// symptom this exists to remove.
		if err := http.NewResponseController(w).SetWriteDeadline(
			time.Now().Add(ai.RouteWriteDeadline),
		); err != nil {
			httperr.Write(w, r, &httperr.DetailedError{
				Status: http.StatusInternalServerError,
				Code:   "deadline_not_extendable",
				Detail: "This server chain cannot extend the response deadline.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// callsAModel reports whether this path is one of the slow AI routes.
func callsAModel(path string) bool {
	for _, suffix := range modelRouteSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
