// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Reading a page of the manual per-record grants.
//
// Split from grants.go because the read answers a different question from the
// writes beside it: those decide whether one share may exist, this one decides
// what a caller may SEE of the shares that do — a page bounded by a keyset, and
// then narrowed again by whether the caller could read each grant's target.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListRecordGrants answers one page of the active manual grants.
//
// PAGED, and this is the one of the three unpaged lists that had to be rather
// than have its dial retired: consent purposes and voice corpus sources are
// configuration a workspace sets up once, but a grant row is created every time
// somebody shares a record. Its count is driven by usage, and the unfiltered
// call read the WHOLE table — then ran one visibility query per row it had read.
//
// THE VISIBILITY FILTER RUNS AFTER THE PAGE, which is what makes the paging
// less obvious than it looks. A page of `limit` rows can serve fewer once the
// invisible ones are dropped, so:
//
//   - `HasMore` comes from the QUERY reading one row beyond the page, never
//     from how many survived the filter — a page that filtered everything out
//     still has more behind it, and reporting otherwise ends the walk early;
//   - the cursor names the last row READ, not the last row served. A cursor
//     minted from the last visible row would re-read every invisible row after
//     it on the next page, forever, without ever serving one.
func (s *Service) ListRecordGrants(ctx context.Context, in ListGrantsInput) ([]grantRow, storekit.Page, error) {
	var out []grantRow
	var page storekit.Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		limit := storekit.ClampLimit(in.Limit)
		candidates, err := s.grantPage(ctx, tx, in, limit)
		if err != nil {
			return err
		}
		if len(candidates) > limit {
			candidates = candidates[:limit]
			last := candidates[len(candidates)-1]
			next, err := storekit.EncodeCursor(last.CreatedAt, last.ID)
			if err != nil {
				return err
			}
			page.NextCursor, page.HasMore = next, true
		}
		out, err = visibleGrants(ctx, tx, candidates)
		return err
	})
	return out, page, err
}

// grantPage reads one page of candidate rows, one beyond the limit so the
// caller can tell a full page from the last one.
func (s *Service) grantPage(ctx context.Context, tx pgx.Tx, in ListGrantsInput, limit int) ([]grantRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := "(expires_at IS NULL OR expires_at > now())"
	for _, f := range []struct {
		column string
		value  any
		set    bool
	}{
		{"record_type", derefOr(in.RecordType), in.RecordType != nil},
		{"record_id", derefOr(in.RecordID), in.RecordID != nil},
		{"subject_type", derefOr(in.SubjectType), in.SubjectType != nil},
		{"subject_id", derefOr(in.SubjectID), in.SubjectID != nil},
	} {
		if f.set {
			where += storekit.SQLf(" AND "+f.column+" = $%d", arg(f.value))
		}
	}
	if in.Cursor != nil && *in.Cursor != "" {
		cursor, err := storekit.DecodeCursor(*in.Cursor)
		if err != nil {
			return nil, err
		}
		where += storekit.SQLf(" AND (created_at, id) < ($%d, $%d)", arg(cursor.CreatedAt), arg(cursor.ID))
	}
	// id joins the ORDER BY so the keyset is TOTAL: two grants created in the
	// same instant would otherwise order arbitrarily, and a cursor over a
	// non-total order repeats or skips at a page boundary.
	//
	// The LIMIT bounds the READ, not just the answer. Truncating in Go after an
	// unbounded query would serve exactly the same page while still pulling the
	// whole table across — and no test through this API can tell the two apart,
	// because the difference is invisible in the result. That is why it is said
	// here rather than left to a test.
	rows, err := tx.Query(ctx,
		"SELECT "+grantColumns+" FROM record_grant WHERE "+where+
			storekit.SQLf(" ORDER BY created_at DESC, id DESC LIMIT $%d", arg(limit+1)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []grantRow
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, g)
	}
	return candidates, rows.Err()
}

// visibleGrants drops the grants whose target the caller cannot read.
//
// Through auth.VisibleSubset, which asks BOTH halves of "could this caller read
// that record": the object grant on the record's type, then its row scope. The
// row-scope probe alone is no answer on the tables every seat reads whole — it
// admits every id without a query — so it is the object grant that keeps a seat
// whose role holds no deal.read from listing every deal share and who holds it.
//
// One statement per record type on the page rather than one per grant, and all
// of it AFTER the page's cursor is drained and closed: each probe is its own
// query on this transaction, and pgx refuses a second query while rows are open
// ("conn busy").
func visibleGrants(ctx context.Context, tx pgx.Tx, candidates []grantRow) ([]grantRow, error) {
	byType := map[string][]ids.UUID{}
	for _, g := range candidates {
		byType[g.RecordType] = append(byType[g.RecordType], g.RecordID)
	}
	readable := make(map[string]map[ids.UUID]bool, len(byType))
	for recordType, recordIDs := range byType {
		subset, err := auth.VisibleSubset(ctx, tx, recordType, recordIDs)
		if err != nil {
			return nil, err
		}
		readable[recordType] = subset
	}
	out := make([]grantRow, 0, len(candidates))
	for _, g := range candidates {
		if readable[g.RecordType][g.RecordID] {
			out = append(out, g)
		}
	}
	return out, nil
}

// derefOr reads a filter's value for binding, or nil when it is unset — the
// caller has already decided whether the clause is rendered at all.
//
//craft:ignore naked-any the four filters bind different types through one argument registrar; the any is the registrar's own
func derefOr[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}
