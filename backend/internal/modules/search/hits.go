// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// hitColumns is what every union element projects and both shapes select, in
// the order scanHits reads it.
const hitColumns = "rtype, id, title, snippet, score, employer_id, employer_name, identifier_match"

// noEmployer is the employer pair of an element that finds records by their own
// text. Typed, because an untyped NULL in the first element of a union resolves
// as text and the arm's uuid then fails the whole statement.
const noEmployer = "NULL::uuid AS employer_id, NULL::text AS employer_name"

// scanHits materializes the rows of either shape's statement.
func scanHits(rows pgx.Rows) ([]Hit, error) {
	var hits []Hit
	for rows.Next() {
		var h Hit
		var title, snippet, employerName *string
		var employerID *ids.UUID
		if err := rows.Scan(&h.Type, &h.ID, &title, &snippet, &h.Score, &employerID, &employerName, &h.IdentifierMatch); err != nil {
			return nil, err
		}
		if employerID != nil && employerName != nil {
			h.WorksAt = &Employer{CompanyID: *employerID, CompanyName: *employerName}
		}
		if title != nil {
			h.Title = *title
		}
		if snippet != nil {
			h.Snippet = strings.TrimSpace(*snippet)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}
