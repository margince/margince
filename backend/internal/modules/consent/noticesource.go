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

// fieldObtainedAt is the body field naming when the contact was obtained.
const fieldObtainedAt = "obtained_at"

// SourceAssessment keeps a human decision separate from the original acquisition evidence.
type SourceAssessment struct {
	Kind       string
	Timezone   string
	ObtainedAt *time.Time
	Note       string
	Now        time.Time
	// MetInPerson says the source was a meeting face to face, which also
	// records the meeting itself (facetoface.go).
	MetInPerson bool
}

func (in SourceAssessment) validate() (NoticeDuty, bool, error) {
	duty, needsCase := DutyFor(in.Kind)
	if duty.State == NoticeSourceReview {
		return duty, false, &ValidationError{Field: auditKeyKind, Reason: "confirm how the contact was obtained"}
	}
	if strings.TrimSpace(in.Note) == "" || utf8.RuneCountInString(in.Note) > noticeNoteLimit {
		return duty, false, &ValidationError{Field: fieldNote, Reason: "give source evidence in 1 to 500 characters"}
	}
	if in.ObtainedAt != nil && (in.ObtainedAt.IsZero() || in.ObtainedAt.After(in.Now)) {
		return duty, false, &ValidationError{Field: fieldObtainedAt, Reason: "give the actual acquisition date, no later than today"}
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
	in.Note = strings.TrimSpace(in.Note)
	duty, needsCase, err := in.validate()
	if err != nil {
		return NoticeCase{}, err
	}
	var out NoticeCase
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if in.MetInPerson {
			if err := holdReviewedContact(ctx, tx, id); err != nil {
				return err
			}
		}
		var err error
		out, err = assessNoticeSourceTx(ctx, tx, id, in, duty, needsCase)
		if err != nil || !in.MetInPerson {
			return err
		}
		return s.recordFaceToFaceTx(ctx, tx, out.ContactID, faceToFaceMeeting{note: in.Note, met: *in.ObtainedAt})
	})
	return out, err
}

// holdReviewedContact holds the contact a review is about before any row of
// theirs is locked. Recording a meeting writes to that contact, and an erasure
// takes the two in this order.
func holdReviewedContact(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var contact ids.UUID
	if err := tx.QueryRow(ctx, `SELECT contact_id FROM privacy_notice_case WHERE id = $1`, id).Scan(&contact); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("consent: reading whom this review is about: %w", err)
	}
	return auth.HoldWritableLive(ctx, tx, entityContact, contact)
}

func assessNoticeSourceTx(ctx context.Context, tx pgx.Tx, id ids.UUID, in SourceAssessment, duty NoticeDuty, needsCase bool) (NoticeCase, error) {
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
	if current.needsSourceReview() && current.State != NoticeSourceReview {
		if _, err := ReviewUnverifiedNoticeTx(ctx, tx, id, in.Now); err != nil {
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
	due, zone, err := in.deadline(duty, needsCase, acquisition.CapturedAt)
	if err != nil {
		return NoticeCase{}, err
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
                completed_at = @completed, resolution_note = @resolution, resolved_by = @actor,
                updated_at = @now WHERE id = @id RETURNING`+noticeCaseColumns,
		pgx.NamedArgs{
			"id": id, fieldState: state, "rule": rule, "due": due, "routes": routes,
			auditKeyKind: in.Kind, "obtained": in.ObtainedAt, fieldNote: in.Note,
			"completed": completed, fieldResolution: resolution, "actor": resolver, "now": in.Now,
		}))
	if err != nil {
		return NoticeCase{}, fmt.Errorf("record the source assessment: %w", err)
	}
	if err := auditNoticeCase(ctx, tx, current, out, map[string]any{
		"source_kind": in.Kind, "source_obtained_at": in.ObtainedAt, "source_review_note": true,
		fieldDueAt: due, "source_timezone": zone,
	}); err != nil {
		return NoticeCase{}, err
	}
	out.Acquisition = acquisition
	return out, nil
}

// ReviewUnverifiedNoticeTx withdraws an unproven deadline while retaining the case and audit history.
func ReviewUnverifiedNoticeTx(ctx context.Context, tx pgx.Tx, id ids.UUID, now time.Time) (bool, error) {
	current, err := lockNoticeCase(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if current.SourceKind != nil || terminalNoticeStates()[current.State] || current.State == NoticeQueued || current.State == NoticeSourceReview {
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
