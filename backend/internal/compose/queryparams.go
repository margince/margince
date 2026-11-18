// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// refuseUndeclaredQueryParameter answers 422 invalid_parameter for a query key
// the operation does not declare. Without it a mistyped filter reads as "no
// filter" and the caller gets the unfiltered list back as if it had been
// narrowed. The table is generated from the contract, so a new parameter is
// admitted by being declared.
func refuseUndeclaredQueryParameter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := undeclaredQueryParameter(r.Method+" "+chiRoutePattern(r), r.URL.RawQuery); err != nil {
			httperr.Write(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// undeclaredQueryParameter names every key of rawQuery that route does not
// declare, in one refusal. A route the contract does not know is left alone.
// A cf_ key is admitted everywhere: custom columns are per workspace, so the
// contract cannot declare them, and the store judges each against the catalog.
func undeclaredQueryParameter(route, rawQuery string) *httperr.DetailedError {
	declared, known := queryParamTable[route]
	if !known || rawQuery == "" {
		return nil
	}
	// Parsed leniently: a malformed pair is the binding's to refuse, and its
	// well-formed siblings are still checked here.
	query, _ := url.ParseQuery(rawQuery)
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

var _ crmcontracts.MiddlewareFunc = refuseUndeclaredQueryParameter
