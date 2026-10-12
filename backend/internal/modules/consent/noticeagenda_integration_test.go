// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A disclosure on its way is not asked for again, and one that bounced is.
// Both stay owed in law; the agenda is what a rep is asked to act on.
func TestASentNoticeLeavesTheAgendaAndABouncedOneReturns(t *testing.T) {
	e := setupChannelConsent(t)
	now := time.Now()
	record := func(acq ids.UUID, state NoticeState) {
		t.Helper()
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
				ContactID: e.contact, AcquisitionID: acq, Rule: RuleArt14, DueAt: now.Add(time.Hour),
				State: state, AllowedRoutes: []string{noticeRouteRecordConfirmation},
			})
		}); err != nil {
			t.Fatalf("recording the duty: %v", err)
		}
	}
	record(seedAcquisition(t, e, "purchased_or_imported"), NoticeOpen)
	if moved := discharge(t, e, now); moved != 1 {
		t.Fatalf("sending the notice queued %d cases, want 1", moved)
	}
	record(seedAcquisition(t, e, "referral"), NoticeDeliveryFailed)

	got, err := e.store.OpenNoticeCasesDueSoonest(privacyOperator(e), NoticeAgendaInput{Limit: 10})
	if err != nil {
		t.Fatalf("reading the agenda: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("the agenda holds %d cases, want only the bounced one: a queued notice is on its way", len(got))
	}
	if state, _ := noticeCaseState(t, e, got[0].ID); state != string(NoticeDeliveryFailed) {
		t.Errorf("the agenda holds a %s case, want the bounced one", state)
	}
}

// The agenda says when each duty was recorded, so a reader can tell a duty
// imported after its deadline from one that fell due while recorded.
func TestTheAgendaSaysWhenADutyWasRecorded(t *testing.T) {
	e := setupChannelConsent(t)
	acq := seedAcquisition(t, e, "unknown_legacy")
	due := time.Date(2020, 2, 20, 0, 0, 0, 0, time.UTC)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{ContactID: e.contact, AcquisitionID: acq, Rule: RuleArt14, DueAt: due})
	}); err != nil {
		t.Fatalf("recording the duty: %v", err)
	}
	got, err := e.store.OpenNoticeCasesDueSoonest(privacyOperator(e), NoticeAgendaInput{Limit: 10})
	if err != nil || len(got) != 1 {
		t.Fatalf("reading the agenda: %d cases, %v", len(got), err)
	}
	if !got[0].OpenedAt.After(got[0].DueAt) {
		t.Errorf("a duty recorded today for a 2020 deadline reads opened %v, due %v", got[0].OpenedAt, got[0].DueAt)
	}
}

// The agenda and the queue say what each deadline rests on: how the contact
// arrived, when, and who recorded it.
func TestADutyCarriesTheAcquisitionItsDeadlineRunsFrom(t *testing.T) {
	e := setupChannelConsent(t)
	acquired := time.Date(2025, 3, 31, 9, 0, 0, 0, time.UTC)
	var acq ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		VALUES ($1, 'referral', $2, $3) RETURNING id`,
		e.contact, acquired, "human:"+e.user.String()).Scan(&acq); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
			ContactID: e.contact, AcquisitionID: acq, Rule: RuleArt14, DueAt: AddMonths(acquired, 1),
		})
	}); err != nil {
		t.Fatalf("recording the duty: %v", err)
	}

	agenda, err := e.store.OpenNoticeCasesDueSoonest(privacyOperator(e), NoticeAgendaInput{Limit: 10})
	if err != nil || len(agenda) != 1 {
		t.Fatalf("reading the agenda: %d cases, %v", len(agenda), err)
	}
	detail, err := e.store.GetNoticeCase(privacyOperator(e), agenda[0].ID)
	if err != nil {
		t.Fatalf("reading the duty: %v", err)
	}
	for read, evidence := range map[string]*NoticeAcquisition{"agenda": agenda[0].Acquisition, "detail": detail.Acquisition} {
		if evidence == nil {
			t.Fatalf("the %s read carries no acquisition, so its deadline reads as authoritative", read)
		}
		if evidence.Kind != "referral" || evidence.OccurredAt == nil || !evidence.OccurredAt.Equal(acquired) {
			t.Errorf("the %s read rests on %s at %v, want a referral on %v", read, evidence.Kind, evidence.OccurredAt, acquired)
		}
		if evidence.CapturedByName == nil || *evidence.CapturedByName != "Rep" {
			t.Errorf("the %s read names the recording seat %v, want its display name", read, evidence.CapturedByName)
		}
	}
}
