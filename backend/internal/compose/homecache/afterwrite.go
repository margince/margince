// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// worklistWrites prefixes the writes that settle or hide a row Home lists.
// They name the row in the body, so the path shows no id.
const worklistWrites = "/v1/worklist"

// hiddenByTheClient lists the writes whose row the app took off Home before
// it sent them. The kept day, flagged stale, serves until the read behind it lands.
// A write that undoes, edits or moves a row stays out: the app cannot draw it.
var hiddenByTheClient = map[string]bool{
	http.MethodPut + " /v1/worklist/done":               true,
	http.MethodPut + " /v1/activities/{id}/disposition": true,
}

// AfterWrite ends the readers' kept Home after r's write answered status.
// A POST that only adds, and a write in hiddenByTheClient, mark the day and Home
// serves it flagged stale. Any other write acts on a row Home may list, so it
// drops the day. A 409 drops it too, so a retry never meets the old version.
func (c *Cache) AfterWrite(r *http.Request, readers []ids.UUID, status int) error {
	return c.end(r.Context(), readers, !keepsTheDay(r, status))
}

func keepsTheDay(r *http.Request, status int) bool {
	route := chi.RouteContext(r.Context())
	if route == nil || status >= http.StatusMultipleChoices {
		return false
	}
	pattern := route.RoutePattern()
	if hiddenByTheClient[r.Method+" "+pattern] {
		return true
	}
	return r.Method == http.MethodPost &&
		!strings.Contains(pattern, "{") &&
		!strings.HasPrefix(pattern, worklistWrites)
}
