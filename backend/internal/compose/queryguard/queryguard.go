// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package queryguard refuses a request whose query string names a key the
// operation does not declare. Without it a mistyped filter reads as no filter,
// and the caller gets the unfiltered list as if it had been narrowed.
package queryguard

import (
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Refuse answers 422 invalid_parameter for a query key the operation does not
// declare. routePattern names the "METHOD /v1/path" template the router matched.
// The table is generated from the contract, so a new parameter is admitted by
// being declared.
func Refuse(routePattern func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := Undeclared(r.Method+" "+routePattern(r), r.URL.RawQuery); err != nil {
				httperr.Write(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Undeclared names every key of rawQuery that route does not declare, in one
// refusal. A route the contract does not list is left alone. A cf_ key is
// admitted everywhere: custom columns are per workspace, so the contract cannot
// declare them, and the store judges each against the catalog.
func Undeclared(route, rawQuery string) *httperr.DetailedError {
	declared, known := Declared[route]
	if !known || rawQuery == "" {
		return nil
	}
	// The parser drops a key with a bad escape. The request would then run
	// unfiltered, so a malformed query is refused whole.
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return httperr.Validation("query", "invalid_parameter", "the query string is malformed; check its percent-escapes")
	}
	var unknown []string
	for key := range query {
		if !slices.Contains(declared, key) && !datasource.IsCustomFieldKey(key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	fields := make([]httperr.FieldError, len(unknown))
	for i, key := range unknown {
		fields[i] = httperr.FieldError{Field: key, Code: "invalid_parameter", Message: "this operation takes no query parameter named " + key}
	}
	return &httperr.DetailedError{
		Status: http.StatusUnprocessableEntity, Code: "validation_error", Fields: fields,
		Detail: "this operation takes no query parameter named " + strings.Join(unknown, ", "),
	}
}
