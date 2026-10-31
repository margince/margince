// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The meetings an SDR has booked for the next two weeks: the count on the
// summary and the list behind it read one predicate, so the list's pages hold
// exactly the count.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// upcomingDays is how many calendar days the upcoming count covers, today
// included.
const upcomingDays = 14

// upcomingWindow runs from at to midnight at the start of the fourteenth day
// after today, in the reporting zone.
func upcomingWindow(ctx context.Context, tx pgx.Tx, at time.Time) (crmcontracts.SdrUpcomingWindow, error) {
	name, err := identity.TimezoneOf(ctx, tx)
	if err != nil {
		return crmcontracts.SdrUpcomingWindow{}, err
	}
	zone, err := time.LoadLocation(name)
	if err != nil {
		return crmcontracts.SdrUpcomingWindow{}, err
	}
	day := at.In(zone)
	end := time.Date(day.Year(), day.Month(), day.Day()+upcomingDays, 0, 0, 0, 0, zone)
	return crmcontracts.SdrUpcomingWindow{StartAt: at, EndAt: end.UTC(), Timezone: name}, nil
}

// upcomingMeetingSQL is the WHERE clause, over activity alias a, of an open
// customer meeting credited to the scope that has not happened by the window's
// start and starts before its end.
func upcomingMeetingSQL(ctx context.Context, tx pgx.Tx, requested RequestedScope, window crmcontracts.SdrUpcomingWindow, b *reportingBindings) (string, error) {
	_, credited, err := analyticsPopulationExpression(ctx, tx, requested, "credit.id", b.arg, unownedIsExcluded)
	if err != nil {
		return "", err
	}
	population := sqlUnnarrowed
	if credited != "" {
		population = "EXISTS (SELECT 1 FROM " + storekit.MeetingCreditedSeatsSQL("a") + " WHERE " + credited + ")"
	}
	visible, err := auth.ActivityContentClause(ctx, "a", b.arg)
	if err != nil {
		return "", err
	}
	fieldScope, _, err := reportingFieldScope(ctx, string(recordTypeActivity), "a", reportingMeetingFields(), b)
	if err != nil {
		return "", err
	}
	upcoming, err := activities.UpcomingMeetingClause(ctx, window.StartAt, b.arg)
	if err != nil {
		return "", err
	}
	return `a.kind='meeting' AND ` + upcoming + ` AND ` + occurredday.BeforeSQL("a", b.add(window.EndAt)) + `
 AND ` + population + ` AND ` + orUnnarrowed(visible) + ` AND ` + fieldScope, nil
}

// ListSdrUpcomingMeetings implements GET /analytics/sdr-summary/meetings.
func (h sdrSummaryHandlers) ListSdrUpcomingMeetings(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSdrUpcomingMeetingsParams) {
	requested := RequestedScope{ID: (*ids.UUID)(params.ScopeId)}
	if params.ScopeKind != nil {
		requested.Kind = string(*params.ScopeKind)
	}
	limit, after, err := sdrPageDials(params.Limit, params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.SdrUpcomingMeetingPage{Rows: []crmcontracts.SdrUpcomingMeeting{}}
	ctx := r.Context()
	err = h.db.Tx(ctx, func(tx pgx.Tx) error {
		out.AsOf = h.now().UTC()
		if _, err := ResolveAnalyticsScope(ctx, tx, requested); err != nil {
			return err
		}
		if !weekly.MayReadMeetings(ctx) {
			return apperrors.ErrPermissionDenied
		}
		var err error
		if out.Window, err = upcomingWindow(ctx, tx, out.AsOf); err != nil {
			return err
		}
		out.Rows, out.NextCursor, err = readUpcomingMeetingPage(ctx, tx, requested, out.Window, after, limit)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// sdrPageDials reads the page size and cursor every SDR summary list takes.
func sdrPageDials(requested *int, cursor *string) (int, *storekit.Cursor, error) {
	limit := sdrHandoffPageDefault
	if requested != nil {
		limit = *requested
	}
	if limit < 1 || limit > sdrHandoffPageMax {
		return 0, nil, httperr.Validation("limit", "out_of_range", fmt.Sprintf("Ask for 1 to %d rows a page.", sdrHandoffPageMax))
	}
	if cursor == nil || *cursor == "" {
		return limit, nil, nil
	}
	position, err := storekit.DecodeCursor(*cursor)
	if err != nil {
		return 0, nil, err
	}
	return limit, &position, nil
}

// readUpcomingMeetingPage reads one page of the meetings readUpcomingMeetings
// counts, soonest first. Each names the first linked customer record the seat
// may read, by that record's own row scope and name mask.
func readUpcomingMeetingPage(ctx context.Context, tx pgx.Tx, requested RequestedScope, window crmcontracts.SdrUpcomingWindow, after *storekit.Cursor, limit int) ([]crmcontracts.SdrUpcomingMeeting, *string, error) {
	var b reportingBindings
	where, err := upcomingMeetingSQL(ctx, tx, requested, window, &b)
	if err != nil {
		return nil, nil, err
	}
	record, err := meetingRecordSQL(ctx, &b)
	if err != nil {
		return nil, nil, err
	}
	page := ""
	if after != nil {
		page = " AND (a.occurred_at,a.id)>(" + b.add(after.CreatedAt) + "," + b.add(after.ID) + ")"
	}
	query := `SELECT a.id,a.occurred_at,a.occurred_precision,COALESCE(a.subject,''),rec.kind,rec.id,rec.label
 FROM activity a LEFT JOIN LATERAL (` + record + `) rec ON true
 WHERE ` + where + page + ` ORDER BY a.occurred_at,a.id LIMIT ` + b.add(limit+1)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, nil, err
	}
	out, err := pgx.CollectRows(rows, scanUpcomingMeeting)
	if err != nil || len(out) <= limit {
		return out, nil, err
	}
	out = out[:limit]
	last := out[limit-1]
	next, err := storekit.EncodeCursor(last.StartsAt, ids.UUID(last.Id))
	if err != nil {
		return nil, nil, err
	}
	return out, &next, nil
}

// meetingRecordSQL picks, for the meeting under alias a, the first linked lead,
// contact or company the seat may read by name, in that order. A kind the seat
// holds no read grant for is never picked, so the row degrades to restricted.
func meetingRecordSQL(ctx context.Context, b *reportingBindings) (string, error) {
	leadVisible, contactVisible, companyVisible := "false", "false", "false"
	var err error
	if auth.ReadGranted(ctx, string(recordTypeLead)) {
		if leadVisible, err = subjectVisibleSQL(ctx, string(recordTypeLead), "l", leadNameFields, b); err != nil {
			return "", err
		}
	}
	if auth.ReadGranted(ctx, tableContact) {
		if contactVisible, err = subjectVisibleSQL(ctx, tableContact, "c", []string{fieldFullName}, b); err != nil {
			return "", err
		}
	}
	if auth.ReadGranted(ctx, tableCompany) {
		if companyVisible, err = subjectVisibleSQL(ctx, tableCompany, "co", []string{companyDisplayName}, b); err != nil {
			return "", err
		}
	}
	label, err := leadLabelSQL(ctx, b, "''")
	if err != nil {
		return "", err
	}
	return `SELECT CASE WHEN l.id IS NOT NULL THEN 'lead' WHEN c.id IS NOT NULL THEN 'contact' ELSE 'company' END AS kind,
 COALESCE(l.id,c.id,co.id) AS id,
 CASE WHEN l.id IS NOT NULL THEN ` + label + ` WHEN c.id IS NOT NULL THEN c.full_name ELSE co.display_name END AS label
 FROM activity_link al
 LEFT JOIN lead l ON l.id=al.lead_id AND ` + leadVisible + `
 LEFT JOIN contact c ON c.id=al.contact_id AND ` + contactVisible + `
 LEFT JOIN company co ON co.id=al.company_id AND ` + companyVisible + `
 WHERE al.activity_id=a.id AND COALESCE(l.id,c.id,co.id) IS NOT NULL
 ORDER BY (l.id IS NULL),(c.id IS NULL),COALESCE(l.id,c.id,co.id) LIMIT 1`, nil
}

func scanUpcomingMeeting(row pgx.CollectableRow) (crmcontracts.SdrUpcomingMeeting, error) {
	var out crmcontracts.SdrUpcomingMeeting
	var kind, label *string
	var record *ids.UUID
	var precision string
	if err := row.Scan(&out.Id, &out.StartsAt, &precision, &out.Title, &kind, &record, &label); err != nil {
		return out, err
	}
	out.StartsPrecision = crmcontracts.SdrUpcomingMeetingStartsPrecision(precision)
	out.Restricted = kind == nil || record == nil
	if !out.Restricted {
		out.Record = &crmcontracts.SdrMeetingRecord{Type: crmcontracts.SdrMeetingRecordType(*kind), Id: *ptrUUID(*record), Label: derefString(label)}
	}
	return out, nil
}
