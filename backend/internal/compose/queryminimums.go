// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"net/http"
	"strconv"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// queryMinimum is one integer query parameter the contract gives a minimum.
type queryMinimum struct {
	Name    string
	Minimum int64
}

// refuseQueryBelowMinimum answers 422 for an integer query value below the
// minimum the contract declares. The stores clamp a page size into range, so
// limit=0 would otherwise answer one row as if it had been asked for. A value
// that is no integer is left to the binding, which refuses it on its own.
func refuseQueryBelowMinimum(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := belowQueryMinimum(r); err != nil {
			httperr.Write(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func belowQueryMinimum(r *http.Request) *httperr.DetailedError {
	minimums := queryMinimumTable[r.Method+" "+chiRoutePattern(r)]
	if len(minimums) == 0 {
		return nil
	}
	query := r.URL.Query()
	for _, m := range minimums {
		for _, raw := range query[m.Name] {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n < m.Minimum {
				return httperr.Validation(m.Name, "out_of_range", fmt.Sprintf("must be at least %d", m.Minimum))
			}
		}
	}
	return nil
}

var _ crmcontracts.MiddlewareFunc = refuseQueryBelowMinimum
