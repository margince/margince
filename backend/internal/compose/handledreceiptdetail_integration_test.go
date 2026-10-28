// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// appliedUnderPolicy decides a staged proposal the way the auto-apply sweep
// does: as an agent carrying the owner's own grants.
func appliedUnderPolicy(ctx context.Context, t *testing.T, svc *approvals.Service, id ids.ApprovalID, owner ids.UUID) {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the reader has no principal to act for")
	}
	actor.Type, actor.ID, actor.OnBehalfOf = principal.PrincipalAgent, autoApplyActorID, owner
	actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)
	if _, err := svc.ApplyUnderPolicy(principal.WithActor(ctx, actor), id); err != nil {
		t.Fatalf("applying under policy: %v", err)
	}
}

// stageDateCard stages a close-date card citing `message`, as the caller.
func (e *closeDateEnv) stageDateCard(ctx context.Context, t *testing.T, deal, message ids.UUID, date time.Time) ids.ApprovalID {
	t.Helper()
	raw, err := json.Marshal(deals.CloseDateCorrection{
		DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: date.Format(time.DateOnly), Asking: deals.AskingIsThisDateRight,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.svc.Stage(ctx, approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, ProposedChange: canonical, DiffHash: hash,
		TargetType: approvalTargetDeal, TargetID: deal, Summary: "Move the close date",
		Evidence: []approvals.Evidence{{Snippet: "Let's sign in the autumn", SourceType: "activity", SourceID: message}},
	})
	if err != nil {
		t.Fatalf("staging the card: %v", err)
	}
	return id
}

func receiptByID(t *testing.T, page crmcontracts.HandledForYou, id ids.UUID) crmcontracts.Receipt {
	t.Helper()
	at := slices.IndexFunc(page.Receipts, func(r crmcontracts.Receipt) bool { return ids.UUID(r.Id) == id })
	if at < 0 {
		t.Fatalf("receipt %s is not on the page: %+v", id, page.Receipts)
	}
	return page.Receipts[at]
}

func changeOf(receipt crmcontracts.Receipt, field string) (crmcontracts.MagicFieldChange, bool) {
	if receipt.Changes == nil {
		return crmcontracts.MagicFieldChange{}, false
	}
	at := slices.IndexFunc(*receipt.Changes, func(c crmcontracts.MagicFieldChange) bool { return c.Field == field })
	if at < 0 {
		return crmcontracts.MagicFieldChange{}, false
	}
	return (*receipt.Changes)[at], true
}

// The sweep's correction and two cards that applied on their own, one staged by
// a member and one by an agent. Each receipt names who did it and what moved.
func TestAReceiptNamesWhoDidItAndWhatItMoved(t *testing.T) {
	e := setupCloseDate(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	swept := e.seedSweepDeal(t, "Swept overnight", e.early, nil, new(-12), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	carded := e.seedSweepDeal(t, "Moved by a card", e.late, new("commit"), new(30), 1)
	subject := "Renewal call"
	message, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "call", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	moved := today().AddDate(0, 0, 45)
	card := e.stageDateCard(reader, t, carded, ids.UUID(message.Id), moved)
	appliedUnderPolicy(reader, t, e.svc, card, e.Rep1)
	agentDeal := e.seedSweepDeal(t, "Moved by an agent", e.late, new("commit"), new(30), 1)
	agent := principal.WithActor(reader, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		PassportID: e.SeedPassport(t, e.owner, "receipt agent"), SeatType: principal.SeatFull,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite), Permissions: integration.AdminPerms,
	})
	agentCard := e.stageDateCard(agent, t, agentDeal, ids.UUID(message.Id), moved)
	appliedUnderPolicy(reader, t, e.svc, agentCard, e.Rep1)

	handled, err := newAttentionService(e.Pool, e.svc, time.Now).HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}

	correction := receiptByID(t, handled, e.correctionAuditIDFor(t, swept))
	if correction.Actor == nil || correction.Actor.Type != crmcontracts.ReceiptActorTypeReceiptActorSystem ||
		correction.Actor.Id != closeDateSweepActor || correction.Actor.Label.Key != "magic.by.system" {
		t.Errorf("the sweep's correction names %+v, want the system close-date job", correction.Actor)
	}
	if change, ok := changeOf(correction, "expected_close_date"); !ok || change.After == nil || change.Before == change.After {
		t.Errorf("the correction's moved close date reads %+v (found=%t)", change, ok)
	}
	if correction.SourceRecord != nil {
		t.Errorf("a correction cites no message, and names %+v", correction.SourceRecord)
	}

	decided := receiptByID(t, handled, card.UUID)
	name := e.WsScalar(t, `SELECT display_name FROM app_user WHERE id = $1`, e.Rep1)
	if decided.Actor == nil || decided.Actor.Type != crmcontracts.ReceiptActorTypeReceiptActorHuman ||
		decided.Actor.Label.Key != "magic.by.member" || decided.Actor.Label.Values == nil ||
		(*decided.Actor.Label.Values)["name"] != name {
		t.Errorf("the card a member staged names %+v, want %q", decided.Actor, name)
	}
	if change, ok := changeOf(decided, "expected_close_date"); !ok || change.After != moved.Format(time.DateOnly) {
		t.Errorf("the applied card's close date reads %+v (found=%t), want it moved to %s", change, ok, moved.Format(time.DateOnly))
	}
	byAgent := receiptByID(t, handled, agentCard.UUID)
	if byAgent.Actor == nil || byAgent.Actor.Type != crmcontracts.ReceiptActorTypeReceiptActorAgent ||
		byAgent.Actor.Id != "agent:test" || byAgent.Actor.Label.Key != "magic.by.agent" ||
		byAgent.Actor.OnBehalfOf == nil || ids.UUID(*byAgent.Actor.OnBehalfOf) != e.Rep1 {
		t.Errorf("the card an agent staged names %+v, want the agent acting for the rep", byAgent.Actor)
	}
	if decided.SourceRecord == nil || ids.UUID(decided.SourceRecord.Id) != ids.UUID(message.Id) ||
		decided.SourceRecord.Label == nil || *decided.SourceRecord.Label != subject {
		t.Errorf("the card's source reads %+v, want the call it was read from", decided.SourceRecord)
	}
}

// A source the reader may no longer open is left off the next read. An earlier
// read that could open it grants nothing.
func TestASourceTheReaderMayNotOpenIsWithheld(t *testing.T) {
	e := setupCloseDate(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	deal := e.seedSweepDeal(t, "Moved by a card", e.late, new("commit"), new(30), 1)
	subject := "Pricing thread"
	message, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "call", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	card := e.stageDateCard(reader, t, deal, ids.UUID(message.Id), today().AddDate(0, 0, 40))
	appliedUnderPolicy(reader, t, e.svc, card, e.Rep1)
	feed := newAttentionService(e.Pool, e.svc, time.Now)

	before, err := feed.HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if receiptByID(t, before, card.UUID).SourceRecord == nil {
		t.Fatal("a source the reader may open is withheld")
	}
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, ids.UUID(message.Id))
	after, err := feed.HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if source := receiptByID(t, after, card.UUID).SourceRecord; source != nil {
		t.Errorf("a source the reader may not open is still named: %+v", source)
	}
}

// A truncated first page continues on the second, which carries every receipt
// the first left out and none that landed after it was read.
func TestATruncatedPageContinuesWhereItStopped(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	now := clocktest.Now(t)
	pipeline, open := signedStage(t, e)
	var svc *approvals.Service
	for i := range 55 {
		svc, _ = appliedStageReceiptIn(t, e, now, fmt.Sprintf("Paged project %02d", i), pipeline, open)
	}
	feed := newAttentionService(e.Pool, svc, func() time.Time { return now })
	first, err := feed.HandledForYou(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextCursor == nil || len(first.Receipts) != 50 {
		t.Fatalf("first page: %d receipts, truncated=%t, cursor=%v; want 50, truncated, a cursor",
			len(first.Receipts), first.Truncated, first.NextCursor)
	}
	_, late := appliedStageReceiptIn(t, e, now, "Landed between pages", pipeline, open)
	second, err := feed.HandledForYou(ctx, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if second.Truncated || second.NextCursor != nil || len(second.Receipts) != 5 {
		t.Fatalf("second page: %d receipts, truncated=%t, cursor=%v; want the 5 left, complete",
			len(second.Receipts), second.Truncated, second.NextCursor)
	}
	seen := map[ids.UUID]bool{}
	for _, receipt := range slices.Concat(first.Receipts, second.Receipts) {
		id := ids.UUID(receipt.Id)
		if seen[id] {
			t.Errorf("receipt %s is on both pages", id)
		}
		seen[id] = true
		if id == late.UUID {
			t.Error("a receipt that landed after the first page shifted the second")
		}
		if receipt.Actor == nil {
			t.Errorf("receipt %s names nobody", id)
		}
		if _, ok := changeOf(receipt, "stage_id"); !ok {
			t.Errorf("stage receipt %s carries no stage change: %+v", id, receipt.Changes)
		}
	}
	if _, err := feed.HandledForYou(ctx, new("not-a-cursor")); err == nil {
		t.Fatal("a cursor this server never minted was read as a page")
	} else if _, malformed := errors.AsType[*storekit.MalformedCursorError](err); !malformed {
		t.Fatalf("a cursor this server never minted answered %v, want the malformed-cursor refusal", err)
	}
}
