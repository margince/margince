// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// Which direction went last on an account, and when, answered for a SET of
// companies in one statement. The page's last-touch section is this over a set
// of one; the ranked queue asks it for every company a page names, so a row and
// the account it opens cannot disagree about who wrote last.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
	roster, err := employeeRoster(ctx, arg)
	if err != nil {
		return nil, err
	}
	// The direction clocks are the roll-up of the employees' own clocks: an
	// employee wrote it, or it reached one. Activity filed on the account alone
	// moves only the last contact. Both skip automatic mail (DirectionClockSQL)
	// and, like the contact page, read only what the whole workspace may see.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		WITH touched AS (
		  SELECT reach.company_id, a.id, a.occurred_at, a.kind,
		         %[5]s AND %[7]s AS they_wrote, %[5]s AND %[8]s AS we_wrote,
		         %[6]s AS is_interaction
		  %[2]s
		), clocks AS (
		  SELECT company_id,
		         max(occurred_at) FILTER (WHERE they_wrote) AS inbound_at,
		         max(occurred_at) FILTER (WHERE we_wrote) AS outbound_at
		  FROM touched GROUP BY company_id
		), last_contact AS (
		  SELECT DISTINCT ON (company_id) company_id, id, occurred_at, kind
		  FROM touched WHERE is_interaction
		  ORDER BY company_id, occurred_at DESC, id DESC
		)
		SELECT %[1]s.id, clocks.inbound_at, clocks.outbound_at, lc.id, lc.occurred_at, lc.kind
		FROM company %[1]s
		LEFT JOIN clocks ON clocks.company_id = %[1]s.id
		LEFT JOIN last_contact lc ON lc.company_id = %[1]s.id
		WHERE %[1]s.id = ANY($%[3]d) AND %[1]s.archived_at IS NULL AND (%[4]s)`,
		lastTouchCompanyAlias, touched, wantedPos, visible,
		relstrength.DirectionClockSQL("a")+" AND "+relstrength.WorkspaceAudienceSQL("a"), relstrength.InteractionCountsSQL("a"),
		employeeSQL(roster, relstrength.TheyWroteSQL("emp.contact_id", "a")),
		employeeSQL(roster, relstrength.WeWroteSQL("a")+" AND "+employeeReachedSQL)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var companyID ids.CompanyID
		var touch LastTouch
		var contactID *ids.ActivityID
		var contactAt *time.Time
		var contactKind *string
		if err := rows.Scan(&companyID, &touch.InboundAt, &touch.OutboundAt,
			&contactID, &contactAt, &contactKind); err != nil {
			return nil, err
		}
		if contactID != nil && contactAt != nil && contactKind != nil {
			touch.Contact = &LastContact{At: *contactAt, Kind: *contactKind, ActivityID: *contactID}
		}
		out[companyID] = touch
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// employeeRoster is the account's employees this caller may read, the roster
// the contacts section is built from. A caller refused it reads no employee,
// so a hidden one moves no clock.
func employeeRoster(ctx context.Context, arg func(any) int) (string, error) {
	roster, err := contacts.CompanyRosterSQL(ctx, "reach.company_id", arg)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return `SELECT NULL::uuid AS contact_id WHERE false`, nil
	}
	return roster, err
}

// employeeSQL admits a row when the predicate holds for one roster employee.
func employeeSQL(roster, predicate string) string {
	return `EXISTS (SELECT 1 FROM (` + roster + `) emp WHERE ` + predicate + `)`
}

// employeeReachedSQL is the contact page's reach for emp.contact_id: the
// activity is linked to them, or they are on it.
const employeeReachedSQL = `(EXISTS (SELECT 1 FROM activity_link el WHERE el.activity_id = a.id AND el.contact_id = emp.contact_id)
		    OR EXISTS (SELECT 1 FROM activity_participant ep WHERE ep.activity_id = a.id AND ep.contact_id = emp.contact_id))`
