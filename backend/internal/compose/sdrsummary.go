// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an SDR is waiting on: handoffs that somebody else must decide, the ones
// sent back to them, and the meetings they have booked. The results are
// reporting metrics in the sdr preset, so this reads only what no metric does.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	limit, after, err := sdrPageDials(params.Limit, params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	status := contacts.HandoffSubmitted
	if params.State != nil && *params.State == crmcontracts.ListSdrWaitingHandoffsParamsStateReturned {
		status = contacts.HandoffRecycled
	}
	out := crmcontracts.SdrWaitingHandoffPage{Rows: []crmcontracts.SdrWaitingHandoff{}}
	ctx := r.Context()
	err = h.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out.AsOf = h.now().UTC()
		out.Rows, out.NextCursor, err = readWaitingHandoffs(ctx, tx, requested, handoffPage{status: status, at: out.AsOf, after: after, limit: limit})
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
	if weekly.MayReadMeetings(ctx) {
		window, err := upcomingWindow(ctx, tx, at)
		if err != nil {
			return out, err
		}
		upcoming, err := readUpcomingMeetings(ctx, tx, requested, window)
		if err != nil {
			return out, err
		}
		out.UpcomingMeetings, out.UpcomingWindow = &upcoming, &window
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

// readUpcomingMeetings counts the meetings listUpcomingMeetings lists, by the
// one predicate both read.
func readUpcomingMeetings(ctx context.Context, tx pgx.Tx, requested RequestedScope, window crmcontracts.SdrUpcomingWindow) (int, error) {
	var b reportingBindings
	where, err := upcomingMeetingSQL(ctx, tx, requested, window, &b)
	if err != nil {
		return 0, err
	}
	var count int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM activity a WHERE `+where, b.values...).Scan(&count)
	return count, err
}

// cycleByAsOf keeps a handoff whose current round began after as_of out of an
// answer stamped as_of. A read-committed statement can see a later commit, and
// its wait would otherwise start after the instant the answer names.
func cycleByAsOf(b *reportingBindings, at time.Time) string {
	return b.add(at) + ">=cycle.occurred_at -- occurred_at raw: a handoff event's instant, not an activity day\n"
}

// handoffPage is one page of open handoffs in one status: submitted and
// waiting on a recipient, or recycled and waiting on the submitter.
type handoffPage struct {
	status string
	at     time.Time
	after  *storekit.Cursor
	limit  int
}

// readWaitingHandoffs reads one page of open handoffs, oldest wait first.
// The prospect's name is read under the prospect's own row scope and field
// mask, so a prospect the seat may not read comes back restricted.
func readWaitingHandoffs(ctx context.Context, tx pgx.Tx, requested RequestedScope, p handoffPage) ([]crmcontracts.SdrWaitingHandoff, *string, error) {
	at, after, limit := p.at, p.after, p.limit
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
 (SELECT count(*) FROM sdr_handoff_event s WHERE s.handoff_id=h.id AND s.to_status=` + b.add(contacts.HandoffSubmitted) + `)>1,
 sv.kind,CASE sv.kind WHEN 'lead' THEN l.id WHEN 'contact' THEN c.id END,
 CASE sv.kind WHEN 'lead' THEN ` + label + ` WHEN 'contact' THEN c.full_name ELSE '' END,
 h.assigned_to,COALESCE(r.display_name,''),COALESCE(` + identity.LiveMemberSQL("r") + `,false),reason.label
 ` + handoffCycleSQL + `
 LEFT JOIN lead l ON l.id=h.lead_id LEFT JOIN contact c ON c.id=h.contact_id
 LEFT JOIN sdr_handoff_reason reason ON reason.id=h.reason_id AND h.status=` + b.add(contacts.HandoffRecycled) + `
 CROSS JOIN LATERAL (SELECT ` + subject + ` AS kind) sv
 WHERE h.status=` + b.add(p.status) + ` AND ` + cycleByAsOf(&b, at) + ` AND ` + orUnnarrowed(population) + page + `
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
	if err := row.Scan(&out.Id, &out.WaitingSince, &submitter, &out.Resubmitted, &subjectType, &subjectID, &label, &recipient, &recipientLabel, &active, &out.Reason); err != nil {
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
