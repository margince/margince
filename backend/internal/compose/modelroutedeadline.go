// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// The routes whose handler calls a model and waits for the answer, matched by
// the suffix their contract path ends in. A model call runs for tens of
// seconds — measured on a real installation, a reply draft takes 13 to 45 —
// and the server's WriteTimeout is 30s for every other endpoint's protection.
//
// Suffix rather than a full path because most of these are mounted under an
// id-bearing prefix (/activities/{id}/draft-email, /companies/{id}/dossier),
// and a list of formatted paths would be a second copy of the router that
// drifts the day a route moves. Suffix matching is also method-blind on
// purpose — the same tradeoff every entry here already makes for its sibling
// GET (on-miss) and POST (always) — so `/brief` also covers the plain re-read
// `GET /v1/brief`, which loses nothing by getting a deadline it does not need.
//
// `/brief` matches three routes, not one: the caller's own morning brief
// (`/v1/brief`) and the contact's and company's own brief endpoints
// (`/contacts/{id}/brief`, `/companies/{id}/brief`) — all three call a
// model and none held this deadline before. `TestTheModelRouteDeadlineSuffixesCoverTheContract`
// (backend/gates) holds this list to the contract's own `x-waits-on-model`
// marker, so a route added there without a matching suffix here fails on its
// own PR rather than waiting for a live 502 to notice.
var modelRouteSuffixes = []string{
	"/draft-email",
	"/dossier",
	"/growth-fit",
	"/meeting-brief",
	"/brief",
	"/ask",
	"/intro-note-draft",
	"/intro-request-draft",
	"/role-proposals",
	"/enrich",
	"/coldstart",
	"/coldstart/preview",
	"/messages",
	"/regenerate",
}

// boundByResponseDeadline decides how long THIS request has, and applies that
// one figure to both halves of it: the response's write deadline, and the
// context the handler behind it runs under.
//
// A model route gets ai.RouteWriteDeadline; every other route keeps the
// chassis's own. The server-wide WriteTimeout stays short on purpose — it
// bounds every endpoint, and a long global deadline lets one slow-reading
// client hold a connection and its handler goroutine for as long as it likes.
// Raising it was the first fix attempted here and it weakens the whole surface
// to help three routes — the same trade the MCP handler already refused
// (modules/agents/httpmcp.go), which extends its own route and says why.
//
// Before the write half, a draft that took longer than 30s had its connection
// cut mid-response. The reader saw a 502 from whatever proxy sits in front: a
// deadline nobody meant to set, reported as a broken server.
//
// The CONTEXT half answers the other direction. Without it a handler whose
// response deadline has passed keeps running: it keeps its goroutine, and — the
// part that costs — it keeps every database connection it takes from there on.
// The worklist family takes 33-54 transactions per request against a pool of
// 16, so a few abandoned reads hold the pool against requests that are still
// answerable.
//
// It cuts nothing that was going to be delivered, because the bound is the one
// the response already ran under. Nor is it a new class of event for a handler
// to survive: r.Context() is ALREADY cancelled when the client disconnects,
// which is this same cancellation on a different trigger. A path that must
// finish regardless says so with context.WithoutCancel and a budget of its own
// — the idiom this tree uses in thirty-eight places, keyvault's detached
// cleanup and compose's logo reclaim among them.
//
// No grace is subtracted to leave room for an error body: at that instant the
// response deadline has passed and the write fails whatever the handler says,
// so a shorter context would stop the work earlier and buy a 504 nobody reads.
func boundByResponseDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !callsAModel(r.URL.Path) {
			serveWithin(w, r, next, httpserver.ResponseDeadline)
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
		serveWithin(w, r, next, ai.RouteWriteDeadline)
	})
}

// serveWithin runs the handler under a context that ends when its response
// deadline does. One helper for both arms, so the two can never name different
// figures from the ones their own responses run under.
func serveWithin(w http.ResponseWriter, r *http.Request, next http.Handler, within time.Duration) {
	ctx, cancel := context.WithTimeout(r.Context(), within)
	defer cancel()
	next.ServeHTTP(w, r.WithContext(ctx))
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
