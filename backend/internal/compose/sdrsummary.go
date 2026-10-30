// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an SDR is waiting on: handoffs that somebody else must decide, the ones
// sent back to them, and the meetings they have booked. The results are
// reporting metrics in the sdr preset, so this reads only what no metric does.

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	sdrHandoffPageDefault = 10
	sdrHandoffPageMax     = 50
)

// handoffCycleSQL joins each handoff to the event that began its current
// state, under the alias cycle. A resubmission writes a new submitted event,
// so a handoff sent back and submitted again waits from that event. It is
// credited to whoever that event names as the submitter.
const handoffCycleSQL = `FROM sdr_handoff h
 JOIN LATERAL (SELECT e.occurred_at,e.submitter_id_at_change FROM sdr_handoff_event e
 WHERE e.handoff_id=h.id AND e.to_status=h.status ORDER BY e.occurred_at DESC,e.id DESC LIMIT 1) cycle ON true
 LEFT JOIN app_user r ON r.id=h.assigned_to`

type sdrSummaryHandlers struct {
	db  *database.DB
	now func() time.Time
}

func newSdrSummaryHandlers(db *database.DB, now func() time.Time) sdrSummaryHandlers {
	return sdrSummaryHandlers{db: db, now: now}
}

// GetSdrSummary implements GET /analytics/sdr-summary.
func (h sdrSummaryHandlers) GetSdrSummary(w http.ResponseWriter, r *http.Request, params crmcontracts.GetSdrSummaryParams) {
	requested := RequestedScope{ID: (*ids.UUID)(params.ScopeId)}
	if params.ScopeKind != nil {
		requested.Kind = string(*params.ScopeKind)
	}
	ctx := r.Context()
	var out crmcontracts.SdrSummary
	err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readSdrSummary(ctx, tx, requested, h.now().UTC())
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListSdrWaitingHandoffs implements GET /analytics/sdr-summary/handoffs.
func (h sdrSummaryHandlers) ListSdrWaitingHandoffs(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSdrWaitingHandoffsParams) {
	requested := RequestedScope{ID: (*ids.UUID)(params.ScopeId)}
	if params.ScopeKind != nil {
		requested.Kind = string(*params.ScopeKind)
	}
	limit := sdrHandoffPageDefault
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < 1 || limit > sdrHandoffPageMax {
		httperr.Write(w, r, httperr.Validation("limit", "out_of_range", fmt.Sprintf("Ask for 1 to %d handoffs a page.", sdrHandoffPageMax)))
		return
	}
	var after *storekit.Cursor
	if params.Cursor != nil && *params.Cursor != "" {
		position, err := storekit.DecodeCursor(*params.Cursor)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		after = &position
	}
	out := crmcontracts.SdrWaitingHandoffPage{Rows: []crmcontracts.SdrWaitingHandoff{}}
	ctx := r.Context()
	err := h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out.AsOf = h.now().UTC()
		out.Rows, out.NextCursor, err = readWaitingHandoffs(ctx, tx, requested, out.AsOf, after, limit)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func readSdrSummary(ctx context.Context, tx pgx.Tx, requested RequestedScope, at time.Time) (crmcontracts.SdrSummary, error) {
	out := crmcontracts.SdrSummary{AsOf: at, Workflow: crmcontracts.SdrSummaryWorkflowQualification}
	if err := auth.Require(ctx, contacts.HandoffObject, principal.ActionRead); err != nil {
		return out, err
	}
	resolved, err := ResolveAnalyticsScope(ctx, tx, requested)
	if err != nil {
		return out, err
	}
	out.Scope = scopeToWire(resolved)
	template, err := reporting.FrameworkTemplate(ctx, tx)
	if err != nil {
		return out, err
	}
	if template == crmcontracts.ReportingFrameworkInputTemplateSdr {
		out.Workflow = crmcontracts.SdrSummaryWorkflowHandoff
		totals, err := readHandoffTotals(ctx, tx, requested, at)
		if err != nil {
			return out, err
		}
		out.Handoffs = &totals
	}
	if auth.Allows(ctx, string(recordTypeActivity), principal.ActionRead) {
		upcoming, err := readUpcomingMeetings(ctx, tx, requested, at)
		if err != nil {
			return out, err
		}
		out.UpcomingMeetings = &upcoming
	}
	return out, nil
}

// readHandoffTotals counts the scope's open handoffs in one statement.
func readHandoffTotals(ctx context.Context, tx pgx.Tx, requested RequestedScope, at time.Time) (crmcontracts.SdrHandoffTotals, error) {
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "cycle.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return crmcontracts.SdrHandoffTotals{}, err
	}
	waiting := "h.status=" + b.add(contacts.HandoffSubmitted)
	query := `SELECT count(*) FILTER (WHERE ` + waiting + `),
 count(*) FILTER (WHERE ` + waiting + ` AND h.assigned_to IS NULL),
 count(*) FILTER (WHERE ` + waiting + ` AND h.assigned_to IS NOT NULL AND NOT (` + identity.LiveMemberSQL("r") + `)),
 count(*) FILTER (WHERE h.status=` + b.add(contacts.HandoffRecycled) + `),
 min(cycle.occurred_at) FILTER (WHERE ` + waiting + `)
 ` + handoffCycleSQL + `
 WHERE h.status IN (` + b.add(contacts.HandoffSubmitted) + `,` + b.add(contacts.HandoffRecycled) + `) AND ` + cycleByAsOf(&b, at) + ` AND ` + orUnnarrowed(population)
	var out crmcontracts.SdrHandoffTotals
	err = tx.QueryRow(ctx, query, b.values...).Scan(&out.Waiting, &out.Unassigned, &out.Blocked, &out.Returned, &out.OldestWaitingSince)
	return out, err
}

// readUpcomingMeetings counts the open customer meetings that have not taken
// place yet, credited to the scope by the rule the held-meetings reading uses.
func readUpcomingMeetings(ctx context.Context, tx pgx.Tx, requested RequestedScope, at time.Time) (int, error) {
	var b reportingBindings
	_, credited, err := analyticsPopulationExpression(ctx, tx, requested, "credit.id", b.arg, unownedIsExcluded)
	if err != nil {
		return 0, err
	}
	population := sqlUnnarrowed
	if credited != "" {
		population = "EXISTS (SELECT 1 FROM " + storekit.MeetingCreditedSeatsSQL("a") + " WHERE " + credited + ")"
	}
	visible, err := auth.ActivityContentClause(ctx, "a", b.arg)
	if err != nil {
		return 0, err
	}
	fieldScope, _, err := reportingFieldScope(ctx, string(recordTypeActivity), "a", reportingMeetingFields(), &b)
	if err != nil {
		return 0, err
	}
	upcoming, err := activities.UpcomingMeetingClause(ctx, at, b.arg)
	if err != nil {
		return 0, err
	}
	query := `SELECT count(*) FROM activity a WHERE a.kind='meeting' AND ` + upcoming + `
 AND ` + population + ` AND ` + orUnnarrowed(visible) + ` AND ` + fieldScope
	var count int
	err = tx.QueryRow(ctx, query, b.values...).Scan(&count)
	return count, err
}

// cycleByAsOf keeps a handoff whose current round began after as_of out of an
// answer stamped as_of. A read-committed statement can see a later commit, and
// its wait would otherwise start after the instant the answer names.
func cycleByAsOf(b *reportingBindings, at time.Time) string {
	return "cycle.occurred_at<=" + b.add(at)
}

// readWaitingHandoffs reads one page of waiting handoffs, oldest wait first.
// The prospect's name is read under the prospect's own row scope and field
// mask, so a prospect the seat may not read comes back restricted.
func readWaitingHandoffs(ctx context.Context, tx pgx.Tx, requested RequestedScope, at time.Time, after *storekit.Cursor, limit int) ([]crmcontracts.SdrWaitingHandoff, *string, error) {
	if err := auth.Require(ctx, contacts.HandoffObject, principal.ActionRead); err != nil {
		return nil, nil, err
	}
	var b reportingBindings
	_, population, err := analyticsPopulationExpression(ctx, tx, requested, "cycle.submitter_id_at_change", b.arg, unownedIsExcluded)
	if err != nil {
		return nil, nil, err
	}
	leadVisible, err := subjectVisibleSQL(ctx, string(recordTypeLead), "l", leadNameFields, &b)
	if err != nil {
		return nil, nil, err
	}
	contactVisible, err := subjectVisibleSQL(ctx, tableContact, "c", []string{fieldFullName}, &b)
	if err != nil {
		return nil, nil, err
	}
	page := ""
	if after != nil {
		page = " AND (cycle.occurred_at,h.id)>(" + b.add(after.CreatedAt) + "," + b.add(after.ID) + ")"
	}
	label, err := leadLabelSQL(ctx, &b, "''")
	if err != nil {
		return nil, nil, err
	}
	subject := "CASE WHEN h.lead_id IS NOT NULL AND " + leadVisible + " THEN 'lead' WHEN h.contact_id IS NOT NULL AND " + contactVisible + " THEN 'contact' END"
	query := `SELECT h.id,cycle.occurred_at,cycle.submitter_id_at_change,
 (SELECT count(*) FROM sdr_handoff_event s WHERE s.handoff_id=h.id AND s.to_status=h.status)>1,
 sv.kind,CASE sv.kind WHEN 'lead' THEN l.id WHEN 'contact' THEN c.id END,
 CASE sv.kind WHEN 'lead' THEN ` + label + ` WHEN 'contact' THEN c.full_name ELSE '' END,
 h.assigned_to,COALESCE(r.display_name,''),COALESCE(` + identity.LiveMemberSQL("r") + `,false)
 ` + handoffCycleSQL + `
 LEFT JOIN lead l ON l.id=h.lead_id LEFT JOIN contact c ON c.id=h.contact_id
 CROSS JOIN LATERAL (SELECT ` + subject + ` AS kind) sv
 WHERE h.status=` + b.add(contacts.HandoffSubmitted) + ` AND ` + cycleByAsOf(&b, at) + ` AND ` + orUnnarrowed(population) + page + `
 ORDER BY cycle.occurred_at,h.id LIMIT ` + b.add(limit+1)
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, nil, err
	}
	out, err := pgx.CollectRows(rows, scanWaitingHandoff)
	if err != nil {
		return nil, nil, err
	}
	if len(out) <= limit {
		return out, nil, nil
	}
	out = out[:limit]
	last := out[limit-1]
	next, err := storekit.EncodeCursor(last.WaitingSince, ids.UUID(last.Id))
	if err != nil {
		return nil, nil, err
	}
	return out, &next, nil
}

// leadNameFields are the columns leadLabelSQL reads, so the label is built only
// from fields whose mask was checked.
var leadNameFields = []string{fieldFullName, companyNameField}

// subjectVisibleSQL is whether this seat may read the prospect under alias by
// name. It checks the row scope and the mask on each column the label reads.
func subjectVisibleSQL(ctx context.Context, table, alias string, nameFields []string, b *reportingBindings) (string, error) {
	rowScope, err := auth.ScopeClauseFor(ctx, table, alias, b.arg)
	if err != nil {
		return "", err
	}
	fieldScope, _, err := reportingFieldScope(ctx, table, alias, nameFields, b)
	if err != nil {
		return "", err
	}
	return "(" + orUnnarrowed(rowScope) + " AND " + fieldScope + ")", nil
}

func scanWaitingHandoff(row pgx.CollectableRow) (crmcontracts.SdrWaitingHandoff, error) {
	var out crmcontracts.SdrWaitingHandoff
	var submitter, subjectID, recipient *ids.UUID
	var subjectType *string
	var label, recipientLabel string
	var active bool
	if err := row.Scan(&out.Id, &out.WaitingSince, &submitter, &out.Resubmitted, &subjectType, &subjectID, &label, &recipient, &recipientLabel, &active); err != nil {
		return out, err
	}
	if submitter != nil {
		out.SubmitterId = ptrUUID(*submitter)
	}
	out.Restricted = subjectType == nil || subjectID == nil
	if !out.Restricted {
		out.Subject = &crmcontracts.SdrHandoffSubject{Type: crmcontracts.SdrHandoffSubjectType(*subjectType), Id: *ptrUUID(*subjectID), Label: label}
	}
	if recipient != nil {
		out.Recipient = &crmcontracts.SdrHandoffRecipient{Id: *ptrUUID(*recipient), Label: recipientLabel, Active: active}
	}
	return out, nil
}
