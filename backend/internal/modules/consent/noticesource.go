// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const fieldObtainedAt = "obtained_at"

// SourceAssessment keeps a human decision separate from the original acquisition evidence.
type SourceAssessment struct {
	Kind             string
	Timezone         string
	ObtainedAt       *time.Time
	Note             string
	Now              time.Time
	SelectedEvidence *SourceEvidenceReference
	PreviewToken     string
	MetInPerson      bool
}

func (in SourceAssessment) validate() (NoticeDuty, bool, error) {
	if in.SelectedEvidence != nil && (!in.SelectedEvidence.valid() || in.SelectedEvidence.ID.IsZero()) {
		return NoticeDuty{}, false, &ValidationError{Field: "selected_evidence", Reason: "select evidence for this contact"}
	}
	duty, needsCase := DutyFor(in.Kind)
	if duty.State == NoticeSourceReview {
		return duty, false, &ValidationError{Field: auditKeyKind, Reason: "confirm how the contact was obtained"}
	}
	if (strings.TrimSpace(in.Note) == "" && in.SelectedEvidence == nil) || utf8.RuneCountInString(in.Note) > noticeNoteLimit {
		return duty, false, &ValidationError{Field: fieldNote, Reason: "give source evidence in 1 to 500 characters"}
	}
	if in.ObtainedAt != nil && (in.ObtainedAt.IsZero() || in.ObtainedAt.After(in.Now)) {
		return duty, false, &ValidationError{Field: "obtained_at", Reason: "give the actual acquisition date, no later than today"}
	}
	if in.MetInPerson && in.Kind != faceToFaceSource {
		return duty, false, &ValidationError{Field: auditKeyKind, Reason: "a meeting face to face is confirmed as permission given in person"}
	}
	if in.MetInPerson && in.ObtainedAt == nil {
		return duty, false, &ValidationError{Field: fieldObtainedAt, Reason: "say when you met them"}
	}
	return duty, needsCase, nil
}

// AssessNoticeSource records a source decision without changing the original
// evidence. The audit row names an agent when one made it.
func (s *Store) AssessNoticeSource(ctx context.Context, id ids.UUID, in SourceAssessment) (NoticeCase, error) {
	if err := requireNoticeCaseUpdate(ctx); err != nil {
		return NoticeCase{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || !actor.HumanOrItsAgent() || actor.UserID.IsZero() {
		return NoticeCase{}, &ValidationError{Field: carrySourceArg, Reason: "a source assessment needs a signed-in privacy officer"}
	}
	in = in.normalized()
	var out NoticeCase
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if in.MetInPerson || in.SelectedEvidence != nil {
			if err := holdReviewedContact(ctx, tx, id, in.MetInPerson); err != nil {
				return err
			}
		}
		var err error
		out, err = assessNoticeSourceTx(ctx, tx, id, in)
		if err != nil || !in.MetInPerson {
			return err
		}
		note := in.Note
		if note == "" && in.SelectedEvidence != nil {
			note = in.SelectedEvidence.Kind + ":" + in.SelectedEvidence.ID.String()
		}
		return s.recordFaceToFaceTx(ctx, tx, out.ContactID, faceToFaceMeeting{note: note, met: *in.ObtainedAt})
	})
	return out, err
}

// The contact is held before its case and activity evidence, following
// erasure's lock order. Selecting a source needs read access only.
func holdReviewedContact(ctx context.Context, tx pgx.Tx, id ids.UUID, recordMeeting bool) error {
	var contact ids.UUID
	if err := tx.QueryRow(ctx, `SELECT contact_id FROM privacy_notice_case WHERE id = $1`, id).Scan(&contact); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("consent: reading whom this review is about: %w", err)
	}
	if recordMeeting {
		return auth.HoldWritableLive(ctx, tx, entityContact, contact)
	}
	if err := auth.EnsureVisibleLive(ctx, tx, entityContact, contact); err != nil {
		return err
	}
	return auth.LockSubjectLive(ctx, tx, entityContact, contact)
}

func assessNoticeSourceTx(ctx context.Context, tx pgx.Tx, id ids.UUID, in SourceAssessment) (NoticeCase, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return NoticeCase{}, &ValidationError{Field: carrySourceArg, Reason: "a source assessment needs a signed-in privacy officer"}
	}
	current, err := lockNoticeCase(ctx, tx, id)
	if err != nil {
		return NoticeCase{}, err
	}
	if err := attachOneAcquisition(ctx, tx, &current); err != nil {
		return NoticeCase{}, err
	}
	acquisition := current.Acquisition
	if acquisition == nil {
		return NoticeCase{}, &ValidationError{Field: carrySourceArg, Reason: "the original acquisition evidence is unavailable"}
	}
	projection, err := prepareSourceProjectionTx(ctx, tx, current, in)
	if err != nil {
		return NoticeCase{}, err
	}
	if in.PreviewToken != "" && in.PreviewToken != projection.PreviewToken {
		return NoticeCase{}, &ValidationError{Field: "preview_token", Reason: "the source assessment changed; preview it again before confirming"}
	}
	duty, needsCase := projection.duty, projection.NoticeRequired
	current, err = sourceCaseForConfirmationTx(ctx, tx, current, in.Now)
	if err != nil {
		return NoticeCase{}, err
	}
	due, zone := projection.DueAt, projection.Timezone
	var evidenceKind *string
	var evidenceID *ids.UUID
	if in.SelectedEvidence != nil {
		evidenceKind, evidenceID = &in.SelectedEvidence.Kind, &in.SelectedEvidence.ID
	}
	var out NoticeCase
	var completed *time.Time
	var resolver *ids.UUID
	state := NoticeExemptWithReason
	var resolution *string
	routes := []string{}
	rule := RuleNone
	if needsCase {
		state, routes, rule = duty.State, duty.Routes, duty.Rule
		if current.OwnerUserID != nil {
			state = NoticeAssigned
		}
	} else {
		ground := "The confirmed source creates no separate notice duty."
		if in.MetInPerson {
			ground = faceToFaceSettled
		}
		completed, resolution, resolver = &in.Now, &ground, &actor.UserID
	}
	out, err = scanNoticeCase(tx.QueryRow(ctx, `UPDATE privacy_notice_case
            SET state = @state, rule = @rule, due_at = @due, allowed_routes = @routes,
                source_kind = @kind, source_obtained_at = @obtained, source_review_note = @note,
                source_evidence_kind = @evidence_kind, source_evidence_id = @evidence_id,
                completed_at = @completed, resolution_note = @resolution, resolved_by = @actor,
                updated_at = @now WHERE id = @id RETURNING`+noticeCaseColumns,
		pgx.NamedArgs{
			"id": id, fieldState: state, "rule": rule, "due": due, "routes": routes,
			auditKeyKind: in.Kind, "obtained": in.ObtainedAt, fieldNote: in.Note,
			"evidence_kind": evidenceKind, "evidence_id": evidenceID,
			"completed": completed, fieldResolution: resolution, "actor": resolver, "now": in.Now,
		}))
	if err != nil {
		return NoticeCase{}, fmt.Errorf("record the source assessment: %w", err)
	}
	if err := auditNoticeCase(ctx, tx, current, out, map[string]any{
		"source_kind": in.Kind, "source_obtained_at": in.ObtainedAt, "source_review_note": true,
		fieldDueAt: due, "source_timezone": zone,
		"source_evidence_kind": evidenceKind, "source_evidence_id": evidenceID,
	}); err != nil {
		return NoticeCase{}, err
	}
	out.Acquisition = acquisition
	return out, nil
}

func sourceCaseForConfirmationTx(ctx context.Context, tx pgx.Tx, current NoticeCase, now time.Time) (NoticeCase, error) {
	acquisition := current.Acquisition
	id := current.ID
	var err error
	if current.needsSourceReview() && current.State != NoticeSourceReview {
		if _, err := ReviewUnverifiedNoticeTx(ctx, tx, id, now); err != nil {
			return NoticeCase{}, err
		}
		current, err = lockNoticeCase(ctx, tx, id)
		if err != nil {
			return NoticeCase{}, err
		}
		current.Acquisition = acquisition
	}
	if current.State != NoticeSourceReview {
		return NoticeCase{}, &ValidationError{Field: fieldState, Reason: "only an unassessed source can be confirmed"}
	}
	return current, nil
}

// ReviewUnverifiedNoticeTx withdraws an unproven deadline while retaining the case and audit history.
func ReviewUnverifiedNoticeTx(ctx context.Context, tx pgx.Tx, id ids.UUID, now time.Time) (bool, error) {
	current, err := lockNoticeCase(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if !current.sourceDecisionPending() {
		return false, nil
	}
	var kind string
	if err := tx.QueryRow(ctx, `SELECT kind FROM contact_acquisition_evidence WHERE id = @acquisition`, pgx.NamedArgs{"acquisition": current.AcquisitionID}).Scan(&kind); err != nil {
		return false, err
	}
	duty, needsCase := DutyFor(kind)
	if !needsCase {
		closed, err := settleAcquisitionCasesTx(ctx, tx, []ids.UUID{current.AcquisitionID}, "The recorded acquisition source creates no separate notice duty.", now)
		return closed > 0, err
	}
	if duty.State != NoticeSourceReview {
		return false, nil
	}
	out, err := scanNoticeCase(tx.QueryRow(ctx, `UPDATE privacy_notice_case
        SET state = 'source_review', rule = @rule, due_at = NULL, allowed_routes = '{}',
            blocked_reason = NULL,
            owner_user_id = `+noticeLiveOwnerSQL("privacy_notice_case")+`,
            updated_at = @now WHERE id = @id RETURNING`+noticeCaseColumns, pgx.NamedArgs{"id": id, "now": now, "rule": duty.Rule}))
	if err != nil {
		return false, err
	}
	err = auditNoticeCase(ctx, tx, current, out, map[string]any{fieldDueAt: nil, "source_unverified": true, fieldOwner: out.OwnerUserID})
	return err == nil, err
}

func (c NoticeCase) sourceDecisionPending() bool {
	return c.SourceKind == nil && !terminalNoticeStates()[c.State] && c.State != NoticeQueued && c.State != NoticeSourceReview
}

func (in SourceAssessment) deadline(duty NoticeDuty, needsCase bool, capturedAt time.Time) (*time.Time, string, error) {
	name := in.Timezone
	if name == "" {
		name = "UTC"
	}
	zone, err := values.ParseTimezone(name)
	if err != nil {
		return nil, "", &ValidationError{Field: "timezone", Reason: "choose an IANA timezone for the acquisition date"}
	}
	location, err := zone.Location()
	if err != nil {
		return nil, "", fmt.Errorf("resolve the acquisition timezone: %w", err)
	}
	if !needsCase {
		return nil, zone.String(), nil
	}
	from := capturedAt
	if in.ObtainedAt != nil {
		from = *in.ObtainedAt
	}
	deadline := AddMonths(from.In(location), duty.Months)
	return &deadline, zone.String(), nil
}
