// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package recordpage maps record identities to canonical application routes.
package recordpage

import (
	"maps"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var routes = map[string]string{"contact": "contacts", "company": "companies", "deal": "deals", "lead": "leads", "project": "projects"}

// Routes returns a copy of the canonical record-page route map.
func Routes() map[string]string { return maps.Clone(routes) }

// URL builds record links from the canonical page routes.
func URL(base, kind string, id ids.UUID) *string {
	route, ok := routes[kind]
	if !ok || base == "" || id.IsZero() {
		return nil
	}
	link := strings.TrimRight(base, "/") + "/#/" + route + "/" + id.String()
	return &link
}
