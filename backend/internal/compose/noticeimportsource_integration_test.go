// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCSVSourceWithoutAnAcquisitionDateKeepsItsNoticeDuty(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	input := contactCreateFrom(map[string]string{"full_name": "Imported Source Example", "email": "source@example.test"}, nil, "csv", "csv_import")
	contact, err := e.Contacts.CreateContact(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	consumer := NewNoticeCaseOpen(e.Pool, func() time.Time { return clocktest.Now(t) }, slog.New(slog.DiscardHandler))
	if err := consumer.HandleEvent(context.Background(), events.Envelope{Type: "contact.created", Entity: events.EntityRef{Type: "contact", ID: ids.UUID(contact.Id)}}); err != nil {
		t.Fatal(err)
	}
	store := consent.NewStore(e.DB())
	cases, _, err := store.ListNoticeCases(ctx, nil, 10, "")
	if err != nil || len(cases) != 1 {
		t.Fatalf("cases=%v err=%v", cases, err)
	}
	duty := cases[0]
	if duty.State != consent.NoticeOpen || duty.Rule != consent.RuleArt14 || duty.Acquisition == nil || duty.Acquisition.Kind != "purchased_or_imported" || duty.Acquisition.OccurredAt != nil {
		t.Fatalf("duty=%+v", duty)
	}
	expected := consent.AddMonths(duty.Acquisition.CapturedAt, 1)
	if duty.DueAt == nil || !duty.DueAt.Equal(expected) {
		t.Fatalf("deadline=%v want=%v", duty.DueAt, expected)
	}
	agenda, err := store.OpenNoticeCasesDueSoonest(ctx, consent.NoticeAgendaInput{ContactID: new(ids.From[ids.ContactKind](ids.UUID(contact.Id)))})
	if err != nil || len(agenda) != 1 || !agenda[0].DeadlineIsUpperBound {
		t.Fatalf("agenda=%v err=%v", agenda, err)
	}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		moved, err := consent.ReviewUnverifiedNoticeTx(ctx, tx, duty.ID, clocktest.Now(t))
		if moved {
			t.Fatal("CSV duty demoted to source review")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
