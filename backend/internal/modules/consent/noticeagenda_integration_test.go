// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
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
