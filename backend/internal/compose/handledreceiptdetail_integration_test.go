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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// appliedUnderPolicy decides a staged proposal the way the auto-apply sweep
// does. It acts as an agent with the owner's own grants, bound again in each
// of the apply's transactions.
func appliedUnderPolicy(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, svc *approvals.Service, id ids.ApprovalID, owner ids.UUID,
) {
	t.Helper()
	applier, decision := ownersApplier(ctx, t, pool, svc, id, owner)
	ownerCtx, err := applier.asOwnersAgent(ctx, owner)
	if err != nil {
		t.Fatalf("binding the owner's agent: %v", err)
	}
	if _, err := svc.ApplyUnderPolicy(ownerCtx, id, applier.ownerNow(decision)); err != nil {
		t.Fatalf("applying under policy: %v", err)
	}
}

// ownersApplier is the production applier over the bound workspace, and its
// decision to apply one approval for owner. The approval may be one an agent
// staged, which the sweep itself never picks up.
func ownersApplier(
	ctx context.Context, t *testing.T, pool *pgxpool.Pool, svc *approvals.Service, id ids.ApprovalID, owner ids.UUID,
) (autoApplier, autoApplyDecision) {
	t.Helper()
	decision := autoApplyDecision{approval: id, owner: owner}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT kind, target_entity_type, target_entity_id FROM approval WHERE id = $1`, id).
			Scan(&decision.target.kind, &decision.target.entityType, &decision.target.entityID)
	}); err != nil {
		t.Fatalf("reading what approval %s targets: %v", id, err)
	}
	return autoApplier{pool: pool, svc: svc, users: identity.NewService(pool)}, decision
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
	carded := e.seedSweepDeal(t, "Moved by a card", e.late, new(95), new(30), 1)
	subject := "Renewal call"
	message, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "call", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	moved := today().AddDate(0, 0, 45)
	card := e.stageDateCard(reader, t, carded, ids.UUID(message.Id), moved)
	appliedUnderPolicy(reader, t, e.Pool, e.svc, card, e.Rep1)
	agentDeal := e.seedSweepDeal(t, "Moved by an agent", e.late, new(95), new(30), 1)
	agent := principal.WithActor(reader, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		PassportID: e.SeedPassport(t, e.owner, "receipt agent"), SeatType: principal.SeatFull,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite), Permissions: integration.AdminPerms,
	})
	agentCard := e.stageDateCard(agent, t, agentDeal, ids.UUID(message.Id), moved)
	appliedUnderPolicy(reader, t, e.Pool, e.svc, agentCard, e.Rep1)

	handled, err := newAttentionService(e.Pool, e.svc, time.Now).HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}

	correction := receiptByID(t, handled, e.correctionAuditIDFor(t, swept))
	if correction.Actor == nil || correction.Actor.Type != crmcontracts.ReceiptActorTypeReceiptActorAgent ||
		correction.Actor.Id != marginceActorID || correction.Actor.Label.Key != "magic.by.agent" ||
		correction.Actor.OnBehalfOf == nil || ids.UUID(*correction.Actor.OnBehalfOf) != e.Rep1 {
		t.Errorf("the sweep's correction names %+v, want Margince acting for the deal's owner", correction.Actor)
	}
	if change, ok := changeOf(correction, "expected_close_date"); !ok || change.After == nil || change.Before == change.After ||
		change.Precision == nil || *change.Precision != crmcontracts.MagicFieldChangePrecisionDay {
		t.Errorf("the correction's moved close date reads %+v (found=%t), want a calendar day", change, ok)
	}
	requireImageKey(t, e.Env, ids.UUID(correction.Id), "close_date_provisional")
	if change, leaked := changeOf(correction, "close_date_provisional"); leaked {
		t.Errorf("the correction lists close_date_provisional: %+v", change)
	}
	if correction.SourceRecord != nil || correction.SourceOccurredAt != nil {
		t.Errorf("a correction cites no message, and names %+v at %v", correction.SourceRecord, correction.SourceOccurredAt)
	}
	if correction.ReasonKind == nil || *correction.ReasonKind != crmcontracts.ReceiptReasonKindRule {
		t.Errorf("the sweep's correction says its reason is %v, want a rule", correction.ReasonKind)
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
	if decided.SourceOccurredAt == nil || !decided.SourceOccurredAt.Equal(message.OccurredAt) {
		t.Errorf("the card's source happened at %v, want the call's own time %v", decided.SourceOccurredAt, message.OccurredAt)
	}
	if decided.ReasonKind != nil {
		t.Errorf("an approval says its reason is %q, want no kind", *decided.ReasonKind)
	}
}

// A verdict change names the message it read and when that came, apart from
// when Margince acted. Home and the deal agree, and call the reason an inference.
func TestAVerdictReceiptNamesTheMessageItReadAndWhenItCame(t *testing.T) {
	v := setupDealVerdict(t)
	v.run(t)
	reader := v.As(v.Rep1, []ids.UUID{v.Team1}, integration.AdminPerms)
	now, h := clocktest.Now(t), newMarginceChangeHandlers(v.Pool)
	handled, err := newAttentionService(v.Pool, approvals.NewService(v.DB()), func() time.Time { return now }).HandledForYou(reader, nil)
	rows, _, readErr := h.deals.DealChanges(reader, ids.From[ids.DealKind](v.deal), nil, 10)
	onDeal, drawErr := h.changeReceipts(reader, rows)
	if err != nil || readErr != nil || drawErr != nil {
		t.Fatal(err, readErr, drawErr)
	}
	verdicts := 0
	for _, receipt := range slices.Concat(handled.Receipts, onDeal) {
		if receipt.Subject == nil || ids.UUID(receipt.Subject.Id) != v.deal {
			continue
		}
		verdicts++
		if at := receipt.SourceOccurredAt; receipt.SourceRecord == nil || receipt.SourceRecord.Id.String() != v.note ||
			at == nil || !at.Equal(v.now) || at.Equal(receipt.OccurredAt) {
			t.Errorf("%s names the source %+v at %v (acted %v), want the buyer's message at %v", receipt.Kind, receipt.SourceRecord, at, receipt.OccurredAt, v.now)
		}
		if precision, kind := receipt.SourceOccurredPrecision, receipt.ReasonKind; precision == nil || *precision != crmcontracts.OccurredPrecisionOccurredPrecisionInstant ||
			kind == nil || *kind != crmcontracts.ReceiptReasonKindInference {
			t.Errorf("%s gives its source precision %v and its reason kind %v, want an instant and an inference", receipt.Kind, precision, kind)
		}
	}
	if verdicts != 6 {
		t.Fatalf("Home and the deal carry %d changes on the deal, want the verdict's three on each", verdicts)
	}
}

// A source the reader may no longer open is left off the next read. An earlier
// read that could open it grants nothing.
func TestASourceTheReaderMayNotOpenIsWithheld(t *testing.T) {
	e := setupCloseDate(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	deal := e.seedSweepDeal(t, "Moved by a card", e.late, new(95), new(30), 1)
	subject := "Pricing thread"
	message, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{Kind: "call", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	card := e.stageDateCard(reader, t, deal, ids.UUID(message.Id), today().AddDate(0, 0, 40))
	appliedUnderPolicy(reader, t, e.Pool, e.svc, card, e.Rep1)
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
	e := setupCloseDate(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	now := clocktest.Now(t)
	subject := "Renewal call"
	message, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "call", Subject: &subject, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	moved := today().AddDate(0, 0, 45)
	applied := func(name string) ids.ApprovalID {
		deal := e.seedSweepDeal(t, name, e.late, new(95), new(30), 1)
		card := e.stageDateCard(ctx, t, deal, ids.UUID(message.Id), moved)
		appliedUnderPolicy(ctx, t, e.Pool, e.svc, card, e.Rep1)
		return card
	}
	for i := range 55 {
		applied(fmt.Sprintf("Paged project %02d", i))
	}
	svc := e.svc
	feed := newAttentionService(e.Pool, svc, func() time.Time { return now })
	first, err := feed.HandledForYou(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextCursor == nil || len(first.Receipts) != 50 {
		t.Fatalf("first page: %d receipts, truncated=%t, cursor=%v; want 50, truncated, a cursor",
			len(first.Receipts), first.Truncated, first.NextCursor)
	}
	late := applied("Landed between pages")
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
		if _, ok := changeOf(receipt, "expected_close_date"); !ok {
			t.Errorf("receipt %s carries no close-date change: %+v", id, receipt.Changes)
		}
	}
	if _, err := feed.HandledForYou(ctx, new("not-a-cursor")); err == nil {
		t.Fatal("a cursor this server never minted was read as a page")
	} else if _, malformed := errors.AsType[*storekit.MalformedCursorError](err); !malformed {
		t.Fatalf("a cursor this server never minted answered %v, want the malformed-cursor refusal", err)
	}
}

// verdictOnAMeeting runs the verdict pass on a deal that cites a captured
// meeting. Then the calendar moves that meeting to a later day.
func verdictOnAMeeting(t *testing.T) (*dealVerdictEnv, time.Time) {
	t.Helper()
	v := setupDealVerdict(t)
	signing := v.now.AddDate(0, 2, 0).Format(time.DateOnly)
	ref, err := calendarSink(v.Env).Upsert(calendarOwnerCtx(v.Env, v.AdminUser), connector.NormalizedRecord{
		EntityType: "activity", NaturalKey: meetingKey,
		Fields: capture.ActivityFields{
			Kind: "meeting", Subject: "Board call", OccurredAt: v.now,
			Body: "Meeting notes: the board agreed we sign on " + signing + " for 45,000 EUR a year.",
		},
		Links:  []datasource.EntityRef{{Type: "deal", ID: v.deal}},
		Source: calendarSystem + ":" + calendarEvent, CapturedBy: "connector:" + calendarSystem,
		Raw: []byte(`{"id":"` + calendarEvent + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	v.note = ref.ID.String()
	v.run(t)
	moved := v.now.AddDate(0, 0, 9)
	if err := movingCalendarSink(v.Env).MoveMeeting(calendarOwnerCtx(v.Env, v.AdminUser), meetingKey,
		connector.MeetingSlot{Start: moved, AllDay: true}); err != nil {
		t.Fatal(err)
	}
	if got := readMeetingStart(t, v.Env, ref.ID); got.Equal(v.now) {
		t.Fatal("the calendar did not move the cited meeting")
	}
	return v, moved
}

// verdictReceiptSources answers each verdict receipt's source date on Home.
func verdictReceiptSources(t *testing.T, v *dealVerdictEnv) []crmcontracts.Receipt {
	t.Helper()
	reader := v.As(v.Rep1, []ids.UUID{v.Team1}, integration.AdminPerms)
	now := clocktest.Now(t)
	handled, err := newAttentionService(v.Pool, approvals.NewService(v.DB()), func() time.Time { return now }).HandledForYou(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out []crmcontracts.Receipt
	for _, receipt := range handled.Receipts {
		if receipt.Subject != nil && ids.UUID(receipt.Subject.Id) == v.deal {
			out = append(out, receipt)
		}
	}
	if len(out) == 0 {
		t.Fatal("Home carries no verdict change on the deal")
	}
	return out
}

// A meeting moved after Margince acted on it leaves the receipt dated as the
// evidence was then. That is the meeting's first start, an instant.
func TestAReceiptKeepsTheDateOfTheMeetingItActedOn(t *testing.T) {
	v, _ := verdictOnAMeeting(t)
	for _, receipt := range verdictReceiptSources(t, v) {
		at, precision := receipt.SourceOccurredAt, receipt.SourceOccurredPrecision
		if receipt.SourceRecord == nil || receipt.SourceRecord.Id.String() != v.note || at == nil || !at.Equal(v.now) ||
			precision == nil || *precision != crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
			t.Errorf("%s names %+v at %v (%v), want the meeting at its first start %v, an instant",
				receipt.Kind, receipt.SourceRecord, at, precision, v.now)
		}
	}
}

// A correction written before the dates were recorded reads the meeting's
// live date. The UPDATE strips the record the real writer wrote, which
// audit_log refuses unless its guard trigger is off for that one statement.
func TestAnOlderReceiptReadsTheMeetingsLiveDate(t *testing.T) {
	v, moved := verdictOnAMeeting(t)
	owner, ctx := integration.OwnerConn(t), context.Background()
	if err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `ALTER TABLE audit_log DISABLE TRIGGER trg_audit_no_mutate;
			UPDATE audit_log SET evidence = evidence - '`+deals.CitedMomentsEvidenceKey+`'
			 WHERE entity_type = 'deal' AND entity_id = '`+v.deal.String()+`' AND evidence ? '`+deals.CitedMomentsEvidenceKey+`';
			ALTER TABLE audit_log ENABLE TRIGGER trg_audit_no_mutate`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, receipt := range verdictReceiptSources(t, v) {
		at, precision := receipt.SourceOccurredAt, receipt.SourceOccurredPrecision
		if at == nil || at.Format(time.DateOnly) != moved.Format(time.DateOnly) ||
			precision == nil || *precision != crmcontracts.OccurredPrecisionOccurredPrecisionDay {
			t.Errorf("%s dates its source %v (%v), want the meeting's live day %s", receipt.Kind, at, precision, moved.Format(time.DateOnly))
		}
	}
}
