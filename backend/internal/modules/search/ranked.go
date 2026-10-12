// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// pageShape is how one request turns its admitted branches into a statement,
// and the rows that statement returns into a page: one ranked list
// (rankedShape), or a few hits of every type (groupedShape).
type pageShape interface {
	statement(branches []string, within string, arg func(any) int) string
	page(hits []Hit) Page
}

// rankedShapeFor reads a ranked request's page, refusing a cursor it cannot
// read before any statement runs.
func rankedShapeFor(in Input) (rankedShape, error) {
	shape := rankedShape{limit: clampLimit(in.Limit)}
	if in.Cursor != "" {
		decoded, err := decodeCursor(in.Cursor)
		if err != nil {
			return rankedShape{}, err
		}
		shape.cursor = &decoded
	}
	return shape, nil
}

// rankedShape is one list ranked across every type, paged by a keyset cursor.
type rankedShape struct {
	cursor *rankedCursor
	limit  int
}

func (r rankedShape) statement(branches []string, within string, arg func(any) int) string {
	var where []string
	if within != "" {
		where = append(where, within)
	}
	if r.cursor != nil {
		// Keyset over the ranked order: strictly worse score, or the same
		// score past the (type, id) tie-break.
		score := arg(r.cursor.Score)
		where = append(where, fmt.Sprintf(
			`(score < $%d OR (score = $%d AND (rtype, id) > ($%d, $%d)))`,
			score, score, arg(r.cursor.Type), arg(r.cursor.ID)))
	}
	sql := "SELECT " + hitColumns + " FROM (" + strings.Join(branches, " UNION ALL ") + ") ranked"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	return sql + fmt.Sprintf(" ORDER BY score DESC, rtype, id LIMIT $%d", arg(r.limit+1))
}

// page derives the keyset cursor from the limit+1 overfetch.
func (r rankedShape) page(hits []Hit) Page {
	page := Page{Hits: hits}
	if len(hits) > r.limit {
		page.Hits = hits[:r.limit]
		page.HasMore = true
		last := page.Hits[r.limit-1]
		page.NextCursor = encodeCursor(rankedCursor{Score: last.Score, Type: last.Type, ID: last.ID})
	}
	return page
}

// rankedCursor is the (score, type, id) keyset position. Encoding keeps
// full float64 precision (strconv 'g' -1) — a rounded score would skip
// or repeat rows on the boundary.
type rankedCursor struct {
	Score float64
	Type  string
	ID    ids.UUID
}

// encodeCursor renders the ranked position. Score, type and id cannot fail to
// marshal; an empty token would be refused on the way back in.
func encodeCursor(c rankedCursor) string {
	token, err := storekit.EncodeOpaque(c)
	if err != nil {
		return ""
	}
	return token
}

func decodeCursor(s string) (rankedCursor, error) {
	c, err := storekit.DecodeOpaque[rankedCursor](s)
	if err != nil {
		return rankedCursor{}, err
	}
	// The envelope proves the token is ours; this proves it names a row. `{}`
	// unmarshals cleanly and leaves a zero id, which would page from a position
	// nothing occupies rather than refuse.
	if c.ID.IsZero() {
		return rankedCursor{}, &storekit.MalformedCursorError{}
	}
	return c, nil
}
