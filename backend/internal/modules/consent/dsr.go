// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Data-subject requests (Art. 15/16/17, B-E11.30): the compliance
// workflow rows the DPO works through. Admin-mediated and human-only at
// the transport (x-agent-access); status transitions demand a
// resolution before a request closes. No dsr.* family exists in the
// events.md closed catalog, so these ride the audit-only lane
// ratified in events.md §5.3c, like the other compliance-config surfaces.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The wire field names a DSR ValidationError carries. A client tells a
// stale-transition refusal apart from a missing-answer one on this exact
// string (details.errors[].field), so they are constants here rather than
// string literals at every raise site — a typo in one raise would answer a
// field name no client is watching for, and nothing would fail.
const (
	fieldKind       = "kind"
	fieldStatus     = "status"
	fieldSubjectRef = "subject_ref"
	fieldResolution = "resolution"
	fieldDueAt      = "due_at"
	fieldAssigneeID = "assignee_id"

	// The request kinds the handlers branch on. Constants for the same reason
	// as the field names above, and dsrKindErasure carries the higher stake:
	// its comparison guards an irreversible scrub, so a typo there skips the
	// erasure silently and reports the request fulfilled.
	dsrKindAccess  = "access"
	dsrKindErasure = "erasure"
)

// illegalTransition is raised from both guards — the pre-erase check and the
// conditional UPDATE that loses a concurrent race — which must name the same
// field and reason, or a client stops recognising the race it already handles.
func illegalTransition(from, to string) *ValidationError {
	return &ValidationError{
		Field:  fieldStatus,
		Reason: from + " → " + to + " is not a legal transition",
	}
}

const dsrColumns = `id, kind, status, subject_ref, assignee_id, due_at, resolution, created_at, contact_id`

// dsrSelectByID is the single-row fetch GetDSR and lockDSR share, so their
// projected columns cannot drift.
const dsrSelectByID = "SELECT " + dsrColumns + " FROM data_subject_request WHERE id = $1"

// lockDSR reads a request FOR UPDATE for both update writers, so the row they
// validate cannot change before they write over it.
func lockDSR(ctx context.Context, tx pgx.Tx, id ids.UUID) (dsrRow, error) {
	current, err := scanDSR(tx.QueryRow(ctx, dsrSelectByID+" FOR UPDATE", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return dsrRow{}, apperrors.ErrNotFound
	}
	return current, err
}

type dsrRow struct {
	// ID is the data_subject_request case id — a compliance workflow row,
	// not a kernel entity, so it stays untyped.
	ID         ids.UUID
	Kind       string
	Status     string
	SubjectRef string
	AssigneeID *ids.UserID
	DueAt      time.Time
	Resolution *string
	CreatedAt  time.Time
	// ContactID is the link a case opened through the subject's own confirm
	// link carries. NIL on one an officer opened by hand, which is why
	// resolveDSRSubject reads the reference as well.
	ContactID *ids.UUID
}

func scanDSR(r pgx.Row) (dsrRow, error) {
	var d dsrRow
	err := r.Scan(&d.ID, &d.Kind, &d.Status, &d.SubjectRef, &d.AssigneeID, &d.DueAt, &d.Resolution,
		&d.CreatedAt, &d.ContactID)
	return d, err
}

// dsrTransitions is the closed status machine: open → in_progress →
// fulfilled|rejected, with a direct open→closed shortcut. A closed
// request never reopens (a new concern is a new request).
var dsrTransitions = map[string]map[string]bool{
	"open":        {"in_progress": true, "fulfilled": true, "rejected": true},
	"in_progress": {"fulfilled": true, "rejected": true},
}

// requireDSRAdmin gates the DSR case queue, and the name is the rule: a
// request row names a data subject (subject_ref is their email or name)
// alongside the statutory deadline and the resolution, so the queue discloses
// who exercised an Art. 15/17 right and what was decided about them.
//
// Admin, not merely an unbounded row scope. Scope answers "which rows may this
// caller see", never "may this caller see this surface" — and three seeded
// roles hold `all`, so an unbounded check handed the whole queue to read_only,
// the least-privileged role in the matrix. Subject-access fulfilment is
// admin-mediated, and reading the queue is how it is mediated.
//
// A human besides: an agent acting under an admin's passport inherits that
// admin's live grants, so without this arm a read-scoped passport would
// enumerate every data subject who ever filed against the workspace.
func requireDSRAdmin(ctx context.Context, action principal.Action) error {
	// ONE object where there were two gates. `contact` plus the literal admin
	// role said "may read contacts, and is an administrator" — two questions
	// standing in for the one nobody could ask: may this caller work the subject
	// queue. privacy_request asks it directly, so an installation delegating the
	// privacy inbox no longer has to hand out member administration with it.
	if err := auth.Require(ctx, "privacy_request", action); err != nil {
		return err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return fmt.Errorf("human-only subject-request queue: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}

// dsrListQuery assembles the keyset-paged queue SQL and its args: an optional
// id > cursor arm, an optional single-status filter, ordered by id with the
// +1 over-fetch ListDSRs uses to detect a further page. A malformed cursor is
// a client error, not an empty result.
func dsrListQuery(cursor, status string, bounded int) (string, []any, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sql := "SELECT " + dsrColumns + " FROM data_subject_request WHERE true"
	if cursor != "" {
		after, err := ids.Parse(cursor)
		if err != nil {
			return "", nil, &storekit.MalformedCursorError{}
		}
		sql += storekit.SQLf(" AND id > $%d", arg(after))
	}
	if status != "" {
		sql += storekit.SQLf(" AND status = $%d", arg(status))
	}
	sql += storekit.SQLf(" ORDER BY id LIMIT $%d", arg(bounded+1))
	return sql, args, nil
}

// collectDSRs drains a queue result set, surfacing a scan or iteration error
// rather than a silent short read.
func collectDSRs(rows pgx.Rows) ([]dsrRow, error) {
	var out []dsrRow
	for rows.Next() {
		d, err := scanDSR(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDSRs walks the case queue newest-id-last. status narrows to one
// queue state ("" = no filter); the contract publishes the filter, so the
// store implements it rather than returning everything.
func (s *Store) ListDSRs(ctx context.Context, limit *int, cursor string, status string) ([]dsrRow, storekit.Page, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	bounded := storekit.ClampLimit(limit)
	var out []dsrRow
	var page storekit.Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		sql, args, err := dsrListQuery(cursor, status, bounded)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		out, err = collectDSRs(rows)
		if err != nil {
			return err
		}
		if len(out) > bounded {
			out = out[:bounded]
			page = storekit.Page{HasMore: true, NextCursor: out[bounded-1].ID.String()}
		}
		return nil
	})
	return out, page, err
}

type CreateDSRInput struct {
	Kind       string
	SubjectRef string
	AssigneeID *ids.UserID
	DueAt      time.Time
}

// CreateDSR files a request into the queue, behind the queue's own gate.
//
// Filing is working the queue, not a lesser act beside it. An erasure request is
// the instruction FulfilErasure later carries out irreversibly, and the officer
// who fulfils it trusts that whoever filed it could. contact.update is not that
// authority: every rep holds it for their own contact edits, and none of them
// may read the queue a request lands in.
func (s *Store) CreateDSR(ctx context.Context, in CreateDSRInput) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	// The kind decides what fulfilling the request does, so an unknown one is
	// refused here rather than stored as a request no path knows how to answer.
	if !crmcontracts.CreateDataSubjectRequestKind(in.Kind).Valid() {
		return dsrRow{}, &ValidationError{Field: fieldKind, Reason: "not a request kind"}
	}
	if strings.TrimSpace(in.SubjectRef) == "" {
		return dsrRow{}, &ValidationError{Field: fieldSubjectRef, Reason: "required"}
	}
	// An absent or null due_at decodes to the zero time, which the queue would
	// sort as overdue since the year 1.
	if in.DueAt.IsZero() {
		return dsrRow{}, &ValidationError{Field: fieldDueAt, Reason: "required"}
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO data_subject_request (kind, subject_ref, assignee_id, due_at)
			VALUES ($1, $2, $3, $4)
			RETURNING `+dsrColumns,
			in.Kind, strings.TrimSpace(in.SubjectRef), in.AssigneeID, in.DueAt)
		var err error
		if out, err = scanDSR(row); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", "data_subject_request", out.ID, nil, map[string]any{
			"kind": in.Kind, fieldSubjectRef: in.SubjectRef, fieldDueAt: in.DueAt,
		})
		return err
	})
	return out, err
}

// GetDSR reads one request, behind the queue's own gate like every other
// entry point here.
func (s *Store) GetDSR(ctx context.Context, id ids.UUID) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanDSR(tx.QueryRow(ctx,
			dsrSelectByID, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	return out, err
}

type UpdateDSRInput struct {
	Status     *string
	AssigneeID *ids.UserID
	Resolution *string
	// ClearAssignee and ClearResolution carry an explicit null, which a nil
	// pointer cannot tell apart from a field the caller left out.
	ClearAssignee   bool
	ClearResolution bool
}

// hasResolution reports whether an update carries (or the row already
// stores) an actual answer — a nil-only check would accept "" or
// whitespace-only text as a resolution, which is exactly the "closing needs
// an answer" rule this guards against.
func hasResolution(value *string) bool {
	return value != nil && strings.TrimSpace(*value) != ""
}

// validateDSRUpdate holds every UpdateDSR precondition: the transition map,
// and a nonblank answer on any request it leaves closed. FulfilErasure runs it
// before the erase, so a request that cannot close is never erased.
func validateDSRUpdate(current dsrRow, in UpdateDSRInput) *ValidationError {
	next := current.Status
	if in.Status != nil && *in.Status != current.Status {
		if !dsrTransitions[current.Status][*in.Status] {
			return illegalTransition(current.Status, *in.Status)
		}
		next = *in.Status
	}
	if !closedDSRStatus(next) {
		return nil
	}
	answer := current.Resolution
	switch {
	case in.ClearResolution:
		answer = nil
	case in.Resolution != nil:
		answer = in.Resolution
	case next == current.Status:
		// A patch that neither closes nor writes the answer leaves the stored one as it is.
		return nil
	}
	if hasResolution(answer) {
		return nil
	}
	if next != current.Status {
		return &ValidationError{Field: fieldResolution, Reason: "closing a request needs its answer"}
	}
	return &ValidationError{Field: fieldResolution, Reason: "a closed request keeps its answer"}
}

// closedDSRStatus reports the two statuses a request ends in.
func closedDSRStatus(status string) bool {
	return status == "fulfilled" || status == "rejected"
}

// dsrAssigneeSet is the assignee assignment both update writers share: an
// explicit clear empties it, an absent value keeps what is stored.
func dsrAssigneeSet(in UpdateDSRInput, arg func(any) int) string {
	return storekit.SQLf("assignee_id = CASE WHEN $%d THEN NULL ELSE coalesce($%d, assignee_id) END",
		arg(in.ClearAssignee), arg(in.AssigneeID))
}

func (s *Store) UpdateDSR(ctx context.Context, id ids.UUID, in UpdateDSRInput) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := lockDSR(ctx, tx, id)
		if err != nil {
			return err
		}
		if verr := validateDSRUpdate(current, in); verr != nil {
			return verr
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		assignee := dsrAssigneeSet(in, arg)
		sql := storekit.SQLf(`
			UPDATE data_subject_request SET
			  status = coalesce($%d, status),
			  %s,
			  resolution = CASE WHEN $%d THEN NULL ELSE coalesce($%d, resolution) END
			WHERE id = $%d
			RETURNING `+dsrColumns, arg(in.Status), assignee, arg(in.ClearResolution), arg(in.Resolution), arg(id))
		if out, err = scanDSR(tx.QueryRow(ctx, sql, args...)); err != nil {
			return err
		}
		return auditDSRUpdate(ctx, tx, current, out, in.Resolution != nil || in.ClearResolution)
	})
	return out, err
}

// auditDSRUpdate is the update audit both writers share. The images carry the
// assignee, so a hand-back says who let the request go.
func auditDSRUpdate(ctx context.Context, tx pgx.Tx, current, out dsrRow, resolutionTouched bool) error {
	_, err := storekit.Audit(ctx, tx, "update", "data_subject_request", out.ID, map[string]any{
		fieldStatus: current.Status, fieldAssigneeID: current.AssigneeID,
	}, map[string]any{
		fieldStatus: out.Status, fieldAssigneeID: out.AssigneeID, fieldResolution: resolutionTouched,
	})
	return err
}

func wireDSR(d dsrRow, subjects map[ids.UUID]dsrSubject) crmcontracts.DataSubjectRequest {
	out := crmcontracts.DataSubjectRequest{
		Id:         openapi_types.UUID(d.ID),
		Kind:       crmcontracts.DataSubjectRequestKind(d.Kind),
		Status:     crmcontracts.DataSubjectRequestStatus(d.Status),
		SubjectRef: d.SubjectRef,
		DueAt:      d.DueAt,
		Resolution: d.Resolution,
		CreatedAt:  d.CreatedAt,
	}
	if d.AssigneeID != nil {
		assignee := openapi_types.UUID(d.AssigneeID.UUID)
		out.AssigneeId = &assignee
	}
	subjectID, ok := resolveDSRSubject(d)
	if !ok {
		return out
	}
	if subject, resolved := subjects[subjectID]; resolved {
		kind := subject.kind
		out.SubjectKind = &kind
		if subject.label != "" {
			label := subject.label
			out.SubjectLabel = &label
		}
	}
	return out
}
