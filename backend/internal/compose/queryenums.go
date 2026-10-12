// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// queryEnum is one query parameter the contract closes to a set of values.
// Array and Explode come from the parameter's own schema and style.
type queryEnum struct {
	Name           string
	Values         []string
	Array, Explode bool
}

// refuseUnknownQueryEnum answers 422 for a query value outside the enum the
// contract declares for it. Binding a value into a generated enum type checks
// nothing, so a mistyped filter would otherwise read as "no rows" or as a
// default. The table is generated from the contract, so a new enum parameter
// is refused by being declared rather than by a handler remembering to.
func refuseUnknownQueryEnum(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := unknownQueryEnum(r); err != nil {
			httperr.Write(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func unknownQueryEnum(r *http.Request) *httperr.DetailedError {
	enums := queryEnumTable[r.Method+" "+chiRoutePattern(r)]
	if len(enums) == 0 {
		return nil
	}
	query := r.URL.Query()
	for _, e := range enums {
		for _, raw := range query[e.Name] {
			if !e.admits(raw) {
				return httperr.Validation(e.Name, "invalid", "expected one of: "+strings.Join(e.Values, ", "))
			}
		}
	}
	return nil
}

// admits reports whether one query value belongs to the enum. Only an array
// that is not exploded travels as one comma-joined value. Anywhere else a comma
// belongs to the value, and the handler reads it as one.
func (e queryEnum) admits(raw string) bool {
	if !e.Array || e.Explode {
		return slices.Contains(e.Values, raw)
	}
	for part := range strings.SplitSeq(raw, ",") {
		if !slices.Contains(e.Values, part) {
			return false
		}
	}
	return true
}

var _ crmcontracts.MiddlewareFunc = refuseUnknownQueryEnum
