// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// WritePicklistRefusal answers a CHECK that holds a field to its options as a
// refusal naming that field, and reports whether err was one. picklists maps a
// module's own core picklist CHECK to its wire field; a custom picklist names
// its own. Every other CHECK is left to Write, which tells the rest apart.
func WritePicklistRefusal(w http.ResponseWriter, r *http.Request, err error, picklists map[string]string) bool {
	field, ok := storekit.PicklistViolation(err)
	if !ok {
		constraint, checked := storekit.CheckViolation(err)
		if field, ok = picklists[constraint]; !checked || !ok {
			return false
		}
	}
	slog.WarnContext(r.Context(), "a picklist value outside its options refused a write",
		"method", r.Method, "path", loggedPath(r), "field", field, "err", err)
	Write(w, r, Validation(field, "value_not_allowed",
		"`"+field+"` takes one of the options this field declares; check the picklist options"))
	return true
}
