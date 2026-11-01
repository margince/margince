// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAssessedSourceControlsTheDisclosureAndSurvivesAutomaticCorrections(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	acquired := time.Date(2020, 1, 31, 10, 0, 0, 0, time.UTC)
	acquisition := seedAcquisition(t, e, "unknown_legacy")
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleUnassessed, State: NoticeSourceReview})
	}); err != nil {
		t.Fatal(err)
	}
	cases, _, err := e.store.ListNoticeCaseReviews(ctx, []NoticeState{NoticeSourceReview}, 10, "")
	if err != nil || len(cases) != 1 {
		t.Fatalf("cases=%v err=%v", cases, err)
	}
	id := cases[0].ID
	if _, err := e.store.AssessNoticeSource(e.ctx, id, SourceAssessment{Kind: "subject_initiated", Note: "Unauthorized assessment", Now: now}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unprivileged assessment: %v", err)
	}
	assigned, err := e.store.AssignNoticeCaseReview(ctx, id, e.user)
	if err != nil || assigned.State != NoticeSourceReview || assigned.DueAt != nil {
		t.Fatalf("assignment=%+v err=%v", assigned, err)
	}
	agenda, err := e.store.OpenNoticeCasesDueSoonest(ctx, NoticeAgendaInput{})
	if err != nil || len(agenda) != 0 {
		t.Fatalf("unconfirmed source reached agenda: %v %v", agenda, err)
	}
	assessed, err := e.store.AssessNoticeSource(ctx, id, SourceAssessment{Kind: "referral", ObtainedAt: &acquired, Note: "Verified source record", Now: now})
	if err != nil || assessed.State != NoticeAssigned {
		t.Fatalf("assessment=%+v err=%v", assessed, err)
	}
	info, err := e.store.PrivacyInformationFor(ctx, ConfirmRef{Kind: LinkPrivacyNotice, ContactID: e.contact})
	if err != nil || info.AcquiredAs != "referral" || info.AcquiredAt == nil || !info.AcquiredAt.Equal(acquired) {
		t.Fatalf("disclosure=%+v err=%v", info, err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		closed, err := SettleWhenSubjectWroteTx(ctx, tx, []ids.UUID{acquisition}, now)
		if err != nil {
			return err
		}
		if closed != 0 {
			t.Fatal("mail capture overwrote the confirmed source")
		}
		moved, err := ReviewUnverifiedNoticeTx(ctx, tx, id, now)
		if moved {
			t.Fatal("correction downgraded a confirmed source")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	final, err := e.store.GetNoticeCaseReview(ctx, id)
	if err != nil || final.State != NoticeAssigned || final.SourceKind == nil || *final.SourceKind != "referral" {
		t.Fatalf("final=%+v err=%v", final, err)
	}
}

func TestUndatedSourceReviewsPageWithoutAStatutoryDeadline(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	for range 2 {
		acquisition := seedAcquisition(t, e, "unknown_legacy")
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleUnassessed, State: NoticeSourceReview})
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, page, err := e.store.ListNoticeCaseReviews(ctx, []NoticeState{NoticeSourceReview}, 1, "")
	if err != nil || len(first) != 1 || !page.HasMore || page.NextCursor == "" {
		t.Fatalf("first=%v page=%+v err=%v", first, page, err)
	}
	second, end, err := e.store.ListNoticeCaseReviews(ctx, []NoticeState{NoticeSourceReview}, 1, page.NextCursor)
	if err != nil || len(second) != 1 || second[0].ID == first[0].ID || end.HasMore {
		t.Fatalf("second=%v page=%+v err=%v", second, end, err)
	}
	if first[0].DueAt != nil || second[0].DueAt != nil {
		t.Fatal("paging invented a legal deadline")
	}
}

func TestUnverifiedLegacyDeadlinesStayOffTheAgendaBeforeCorrection(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	due := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, kind := range []string{"unknown_legacy", "referral"} {
		acquisition := seedAcquisition(t, e, kind)
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			if kind == "referral" {
				if _, err := tx.Exec(ctx, `UPDATE contact_acquisition_evidence SET occurred_at = NULL WHERE id = @id`, pgx.NamedArgs{"id": acquisition}); err != nil {
					return err
				}
			}
			return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleArt14, DueAt: due})
		}); err != nil {
			t.Fatal(err)
		}
	}
	agenda, err := e.store.OpenNoticeCasesDueSoonest(ctx, NoticeAgendaInput{})
	if err != nil || len(agenda) != 1 {
		t.Fatalf("unverified deadlines reached the agenda: %v %v", agenda, err)
	}
	cases, _, err := e.store.ListNoticeCaseReviews(ctx, nil, 10, "")
	if err != nil || len(cases) != 2 {
		t.Fatalf("legacy cases were lost: %v %v", cases, err)
	}
}

func TestUnverifiedLegacyCasesUseTheReviewSurfaceBeforeCorrection(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	acquisition := seedAcquisition(t, e, "unknown_legacy")
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleArt14, DueAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	reviews, _, err := e.store.ListNoticeCaseReviews(ctx, []NoticeState{NoticeSourceReview}, 10, "")
	if err != nil || len(reviews) != 1 {
		t.Fatalf("reviews=%v err=%v", reviews, err)
	}
	id := reviews[0].ID
	if !reviews[0].needsSourceReview() {
		t.Fatal("legacy unknown source was treated as established")
	}
	if _, err := e.store.GetNoticeCase(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("legacy get=%v", err)
	}
	duties, _, err := e.store.ListNoticeCases(ctx, nil, 10, "")
	if err != nil || len(duties) != 0 {
		t.Fatalf("duties=%v err=%v", duties, err)
	}
	if _, err := e.store.ExcuseNoticeCase(ctx, id, ExcuseInput{State: NoticeExemptWithReason, Note: "Bypass assessment", Now: now}); err == nil {
		t.Fatal("unestablished duty accepted excuse")
	}
	assessed, err := e.store.AssessNoticeSource(ctx, id, SourceAssessment{Kind: "referral", Note: "Verified referral, original date unavailable", Now: now})
	if err != nil || assessed.State != NoticeOpen || !assessed.deadlineIsUpperBound() {
		t.Fatalf("assessment=%+v err=%v", assessed, err)
	}
	expected := AddMonths(assessed.Acquisition.CapturedAt, 1)
	if assessed.DueAt == nil || !assessed.DueAt.Equal(expected) {
		t.Fatalf("bound=%v expected=%v", assessed.DueAt, expected)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		closed, err := SettleWhenCaptureWithdrewTx(ctx, tx, e.contact.UUID, now)
		if closed != 0 {
			t.Fatal("withdrawal overwrote human source evidence")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSourceReviewCannotBeExcusedWithoutAnAssessment(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	acquisition := seedAcquisition(t, e, "unknown_legacy")
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleUnassessed, State: NoticeSourceReview})
	}); err != nil {
		t.Fatal(err)
	}
	reviews, _, err := e.store.ListNoticeCaseReviews(ctx, nil, 10, "")
	if err != nil || len(reviews) != 1 {
		t.Fatalf("reviews=%v err=%v", reviews, err)
	}
	if _, err := e.store.ExcuseNoticeCase(ctx, reviews[0].ID, ExcuseInput{State: NoticeProvidedElsewhere, Note: "No source evidence", Now: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}); err == nil {
		t.Fatal("source review was excused")
	}
}

func TestUndatedIndirectAcquisitionSurvivesAutomaticCorrections(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	acquisition := seedAcquisition(t, e, "purchased_or_imported")
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE contact_acquisition_evidence SET occurred_at = NULL WHERE id = @id`, pgx.NamedArgs{"id": acquisition}); err != nil {
			return err
		}
		return OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleArt14, DueAt: now})
	}); err != nil {
		t.Fatal(err)
	}
	duties, _, err := e.store.ListNoticeCases(ctx, nil, 10, "")
	if err != nil || len(duties) != 1 || !duties[0].deadlineIsUpperBound() {
		t.Fatalf("duties=%v err=%v", duties, err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		moved, err := ReviewUnverifiedNoticeTx(ctx, tx, duties[0].ID, now)
		if err != nil {
			return err
		}
		if moved {
			t.Fatal("confirmed source demoted")
		}
		closed, err := SettleWhenCaptureWithdrewTx(ctx, tx, e.contact.UUID, now)
		if err != nil {
			return err
		}
		if closed != 0 {
			t.Fatal("known indirect source withdrawn")
		}
		closed, err = SettleWhenSubjectWroteTx(ctx, tx, []ids.UUID{acquisition}, now)
		if closed != 0 {
			t.Fatal("known indirect source settled by mail")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownSourceBounceReturnsToReviewAndClearsARetiredOwner(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	id := seedNoticeCase(t, e, "unknown_legacy", []string{noticeRouteRecordConfirmation})
	delivery := seedDelivery(t, e)
	at := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE privacy_notice_case SET owner_user_id = @owner WHERE id = @id`, pgx.NamedArgs{"owner": e.user, "id": id}); err != nil {
			return err
		}
		_, err := dischargeNoticeCases(ctx, tx, e.contact, noticeRouteRecordConfirmation, at, delivery)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE app_user SET status = 'deactivated', removed_at = @at WHERE id = @id`, pgx.NamedArgs{"id": e.user, "at": at}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		count, err := MarkNoticeDeliveryFailedTx(ctx, tx, delivery)
		if err == nil && count != 1 {
			t.Fatalf("delivery outcomes=%d", count)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	review, err := e.store.GetNoticeCaseReview(ctx, id)
	if err != nil || review.State != NoticeSourceReview || review.OwnerUserID != nil || review.DueAt != nil || len(review.AllowedRoutes) != 0 {
		t.Fatalf("review=%+v err=%v", review, err)
	}
}

func TestMixedQueuePaginationKeepsItsPositionAcrossSourceCorrection(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := officerCtx(e)
	legalDue := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	reviewCreated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, kind := range []string{"referral", "unknown_legacy", "unknown_legacy"} {
		acquisition := seedAcquisition(t, e, kind)
		due := legalDue
		if kind == "unknown_legacy" {
			due = legalDue.AddDate(-1, 0, 0)
		}
		if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			if err := OpenNoticeCaseTx(ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleArt14, DueAt: due}); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE privacy_notice_case SET created_at = @created WHERE acquisition_id = @id`, pgx.NamedArgs{"id": acquisition, "created": reviewCreated})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	first, page, err := e.store.ListNoticeCaseReviews(ctx, nil, 1, "")
	if err != nil || len(first) != 1 || first[0].needsSourceReview() || !page.HasMore {
		t.Fatalf("first=%v page=%+v err=%v", first, page, err)
	}
	reviews, _, err := e.store.ListNoticeCaseReviews(ctx, []NoticeState{NoticeSourceReview}, 10, "")
	if err != nil || len(reviews) != 2 {
		t.Fatalf("reviews=%v err=%v", reviews, err)
	}
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, review := range reviews {
			if _, err := ReviewUnverifiedNoticeTx(ctx, tx, review.ID, reviewCreated); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	second, next, err := e.store.ListNoticeCaseReviews(ctx, nil, 1, page.NextCursor)
	if err != nil || len(second) != 1 || !second[0].needsSourceReview() || !next.HasMore {
		t.Fatalf("second=%v page=%+v err=%v", second, next, err)
	}
	third, end, err := e.store.ListNoticeCaseReviews(ctx, nil, 1, next.NextCursor)
	if err != nil || len(third) != 1 || third[0].ID == second[0].ID || !third[0].needsSourceReview() || end.HasMore {
		t.Fatalf("third=%v page=%+v err=%v", third, end, err)
	}
}
