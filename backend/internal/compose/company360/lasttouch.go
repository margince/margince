// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Which direction went last on an account, and when, answered for a SET of
// companies in one statement. The page's last-touch section is this over a set
// of one; the ranked queue asks it for every company a page names, so a row and
// the account it opens cannot disagree about who wrote last.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// LastTouch is when the account last wrote to us and when we last wrote to it.
// Nil on either side means it never happened; a withheld answer is a refusal
// from LastTouchFor, never a zero in its map.
type LastTouch struct {
	InboundAt  *time.Time
	OutboundAt *time.Time
	// The precisions of the rows behind InboundAt and OutboundAt: "day" when
	// that row names only a date, empty when there is no row.
	InboundPrecision, OutboundPrecision string
	// Contact is the newest exchange of any direction that counts as contact
	// (relstrength.InteractionCountsSQL): a note is not contact, and neither is
	// a meeting called off. Nil when there was none.
	Contact *LastContact
}

// LastContact names the activity behind LastTouch.Contact, so a reader can
// point at the same row the date came from.
type LastContact struct {
	At         time.Time
	Kind       string
	ActivityID ids.ActivityID
	// Precision is "day" when At is only a date's anchor.
	Precision string
}

// lastTouchCompanyAlias is the company alias LastTouchFor's statement and its
// company row scope share.
const lastTouchCompanyAlias = "co"

// LastTouchFor answers the pair that replaced the header's 0-100 score
// (AC-company-2, ADR-0079 arc) for every company in the set the caller may
// read. A company the caller may not read, or one that is archived, is absent;
// a caller who may not read activity is refused outright.
//
// Two timestamps rather than one "last touch", because which side wrote last
// IS the question: an account we mailed a fortnight ago with no reply and one
// that wrote to us this morning have the same last-touch date and opposite
// meanings.
//
// It walks the links the timeline does, through the timeline predicate's set
// form, so the header can never disagree with the list under it. It carries the
// caller's activity row scope, so a rep sees the last message THEY may read
// rather than the account's true last one.
// A message dated after now has not been sent yet, so it is neither side's last
// word: a scheduled send must not hide the real last one.
func LastTouchFor(
	ctx context.Context, tx pgx.Tx, companyIDs []ids.CompanyID, now time.Time, opts AssembleOptions,
) (map[ids.CompanyID]LastTouch, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.CompanyID]LastTouch, len(companyIDs))
	if len(companyIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	wantedPos := arg(companyIDs)
	visible, err := scopeClause(ctx, "company", lastTouchCompanyAlias, arg)
	if err != nil {
		return nil, err
	}
	scope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	// The walk starts from the accounts and is aggregated once. Its cost is the
	// page's own activity, not how far back a quiet account's last message is.
	touched := "FROM (" + activities.CompanyReachedActivities(wantedPos) + ") reach " +
		"JOIN activity a ON a.id = reach.activity_id WHERE a.archived_at IS NULL"
	if scope != "" {
		touched += " AND " + scope
	}
	touched += opts.projectScope(arg)
	touched += fmt.Sprintf(" AND a.occurred_at <= $%d", arg(now))
	// Both direction clocks skip a called-off meeting and automatic mail, as
	// the last contact does through InteractionCountsSQL. DirectionClockSQL
	// rather than InteractionCountsSQL: a direction clock answers when a
	// human's message last went each way, so any directed kind counts.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH touched AS (
		  SELECT reach.company_id, a.id, a.occurred_at, a.occurred_precision, a.kind, a.direction,
		         %[5]s AS held, %[6]s AS is_interaction
		  %[2]s
		), clocks AS (
		  SELECT company_id,
		         max(occurred_at) FILTER (WHERE direction = 'inbound' AND held) AS inbound_at,
		         max(occurred_at) FILTER (WHERE direction = 'outbound' AND held) AS outbound_at,
		         (array_agg(occurred_precision ORDER BY occurred_at DESC, id DESC)
		            FILTER (WHERE direction = 'inbound' AND held))[1] AS inbound_precision,
		         (array_agg(occurred_precision ORDER BY occurred_at DESC, id DESC)
		            FILTER (WHERE direction = 'outbound' AND held))[1] AS outbound_precision
		  FROM touched GROUP BY company_id
		), last_contact AS (
		  SELECT DISTINCT ON (company_id) company_id, id, occurred_at, occurred_precision, kind
		  FROM touched WHERE is_interaction
		  ORDER BY company_id, occurred_at DESC, id DESC
		)
		SELECT %[1]s.id, clocks.inbound_at, clocks.outbound_at,
		       coalesce(clocks.inbound_precision, ''), coalesce(clocks.outbound_precision, ''),
		       lc.id, lc.occurred_at, lc.kind, coalesce(lc.occurred_precision, '')
		FROM company %[1]s
		LEFT JOIN clocks ON clocks.company_id = %[1]s.id
		LEFT JOIN last_contact lc ON lc.company_id = %[1]s.id
		WHERE %[1]s.id = ANY($%[3]d) AND %[1]s.archived_at IS NULL AND (%[4]s)`,
		lastTouchCompanyAlias, touched, wantedPos, visible,
		relstrength.DirectionClockSQL("a"), relstrength.InteractionCountsSQL("a")), args...)
	if err != nil {
		return nil, err
	}
	return scanLastTouches(rows, out)
}

// scanLastTouches reads LastTouchFor's rows into out, one company each.
func scanLastTouches(rows pgx.Rows, out map[ids.CompanyID]LastTouch) (map[ids.CompanyID]LastTouch, error) {
	defer rows.Close()
	for rows.Next() {
		var companyID ids.CompanyID
		var touch LastTouch
		var contactID *ids.ActivityID
		var contactAt *time.Time
		var contactKind *string
		var contactPrecision string
		if err := rows.Scan(&companyID, &touch.InboundAt, &touch.OutboundAt,
			&touch.InboundPrecision, &touch.OutboundPrecision,
			&contactID, &contactAt, &contactKind, &contactPrecision); err != nil {
			return nil, err
		}
		if contactID != nil && contactAt != nil && contactKind != nil {
			touch.Contact = &LastContact{At: *contactAt, Kind: *contactKind, ActivityID: *contactID, Precision: contactPrecision}
		}
		out[companyID] = touch
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
