// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package finance

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dunningSource is the fixture ledger with reminder counts stated by number.
// An invoice it does not name keeps the fixture's nil level.
type dunningSource struct {
	fixtureSource
	levels map[string]int
}

func (s dunningSource) InvoicesFor(ctx context.Context, customer string) (SourceLedger, error) {
	ledger, err := s.fixtureSource.InvoicesFor(ctx, customer)
	if err != nil {
		return SourceLedger{}, err
	}
	for i := range ledger.Invoices {
		if level, ok := s.levels[ledger.Invoices[i].Number]; ok {
			ledger.Invoices[i].DunningLevel = &level
		}
	}
	return ledger, nil
}

// At the epoch INV-036 is twelve days past due and INV-039 is not due yet. The
// warning reads only overdue invoices, so INV-039's higher count must not show.
func TestTheWarningCarriesTheHighestReminderOnAnOverdueInvoice(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	unstated := e.summaryAtEpoch(t, e.company)
	if unstated.OverdueCount == nil || *unstated.OverdueCount == 0 {
		t.Fatalf("the fixture has nothing overdue at the epoch (count %v); nothing below is tested", unstated.OverdueCount)
	}
	if unstated.OldestOverdueDays == nil || *unstated.OldestOverdueDays == 0 {
		t.Fatalf("an overdue account reports no oldest age (%v)", unstated.OldestOverdueDays)
	}
	if unstated.DunningLevel != nil {
		t.Fatalf("a source that states no dunning level reads level %d", *unstated.DunningLevel)
	}

	stated := dunningSource{levels: map[string]int{"INV-036": 2, "INV-035": 1, "INV-039": 5}}
	pass, err := e.store.SyncConnection(e.ctx, stated)
	if err != nil {
		t.Fatalf("sync with dunning levels: %v", err)
	}
	if pass.InvoicesUpdate != len(stated.levels) {
		t.Errorf("stating %d levels rewrote %d invoices", len(stated.levels), pass.InvoicesUpdate)
	}
	card := e.summaryAtEpoch(t, e.company)
	if card.DunningLevel == nil || *card.DunningLevel != 2 {
		t.Fatalf("dunning level %v, want 2 from INV-036, the overdue invoice with the most reminders", card.DunningLevel)
	}
	if again, err := e.store.SyncConnection(e.ctx, stated); err != nil || again.InvoicesUpdate != 0 {
		t.Fatalf("a second pass over the same levels rewrote %d invoices (err %v)", again.InvoicesUpdate, err)
	}
}

func TestADunningLevelOutsideTheRangeIsRefused(t *testing.T) {
	e := setupFinance(t)
	_, err := e.store.SyncConnection(e.ctx, dunningSource{levels: map[string]int{"INV-036": maxDunningLevel + 1}})
	if err == nil {
		t.Fatal("a level above the range was mirrored")
	}
}

// The warning rides the finance card's own read rule: no finance read, no
// figure, and a refusal rather than an empty card.
func TestAReaderWithoutFinanceReadGetsNoWarning(t *testing.T) {
	e := setupFinance(t)
	if _, err := e.store.SyncConnection(e.ctx, e.provider()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	reader := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:no-finance",
		Permissions: principal.Permissions{
			RoleKeys: []string{"member"},
			Objects:  map[string]principal.ObjectGrant{"company": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	card, err := e.store.SummaryFor(reader, e.company)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err %v, want permission denied", err)
	}
	if card.Overdue != nil || card.OverdueCount != nil {
		t.Fatalf("a refused read still carries the overdue figures: %+v", card)
	}
}

// INV-020 is disputed and long past due. It stays in the open balance and out
// of every overdue figure, its reminders included; INV-036 is the one overdue.
func TestADisputedInvoiceIsOwedButNotOverdue(t *testing.T) {
	e := setupFinance(t)
	stated := dunningSource{levels: map[string]int{"INV-020": 7, "INV-036": 1}}
	if _, err := e.store.SyncConnection(e.ctx, stated); err != nil {
		t.Fatalf("sync: %v", err)
	}
	card := e.summaryAtEpoch(t, e.company)
	if card.OverdueCount == nil || *card.OverdueCount != 1 {
		t.Fatalf("overdue count %v, want 1: the disputed invoice is not overdue", card.OverdueCount)
	}
	if card.OldestOverdueDays == nil || *card.OldestOverdueDays != 12 {
		t.Fatalf("oldest overdue %v days, want 12 from INV-036", card.OldestOverdueDays)
	}
	if card.DunningLevel == nil || *card.DunningLevel != 1 {
		t.Fatalf("dunning level %v, want 1: the disputed invoice's 7 must not show", card.DunningLevel)
	}
	// Open less overdue is INV-020's gross plus the three invoices not yet due:
	// 208800 + 284280 + 288720 + 293160.
	const owedNotOverdue = 1074960
	if card.OpenBalance == nil || card.Overdue == nil ||
		*card.OpenBalance.AmountMinor-*card.Overdue.AmountMinor != owedNotOverdue {
		t.Fatalf("open %+v less overdue %+v is not %d; the disputed invoice left the open balance",
			card.OpenBalance, card.Overdue, owedNotOverdue)
	}
}
