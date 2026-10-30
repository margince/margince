// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A colleague who can see a contact or company but not change it asks its
// owner to hand it over. The owner answers on their Worklist.

import (
	"context"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// handoverRepPerms is a rep on their own row scope who may change contacts,
// companies, deals and tasks. A colleague's record is not theirs to change.
func handoverRepPerms() principal.Permissions {
	perms := integration.AccountRepPerms
	perms.Objects = maps.Clone(integration.AccountRepPerms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	perms.RowScope = principal.RowScopeOwn
	return perms
}

// seatedAs is a signed-in colleague on a full seat.
func seatedAs(e *integration.Env, user ids.UUID, perms principal.Permissions) context.Context {
	ctx := e.As(user, nil, perms)
	p, _ := principal.Actor(ctx)
	p.SeatType = principal.SeatFull
	return principal.WithActor(ctx, p)
}

func asker(e *integration.Env) context.Context { return seatedAs(e, e.Rep3, handoverRepPerms()) }
func giver(e *integration.Env) context.Context { return seatedAs(e, e.Rep1, handoverRepPerms()) }

func requestHandover(t *testing.T, e *integration.Env, ctx context.Context, recordType string, id ids.UUID) (ownershipRequestView, bool) {
	t.Helper()
	out, created, err := newOwnershipTransfers(e.Pool).Request(ctx, recordType, id, "I run this account now")
	if err != nil {
		t.Fatalf("asking for the %s: %v", recordType, err)
	}
	return out, created
}

func decideHandover(t *testing.T, e *integration.Env, ctx context.Context, id ids.ApprovalID, approve bool) error {
	t.Helper()
	_, err := approvalsServiceWithEffects(e.Pool).Decide(ctx, id, approve, nil)
	return err
}

func transfersInInbox(t *testing.T, e *integration.Env, reader context.Context) int {
	t.Helper()
	kind, pending := ownershipTransferKind, approvals.StatusPending
	rows, _, err := approvalsServiceWithEffects(e.Pool).ListWire(reader, approvals.ListInput{Kind: &kind, Status: &pending})
	if err != nil {
		t.Fatalf("reading the inbox: %v", err)
	}
	return len(rows)
}

func noticesOf(t *testing.T, e *integration.Env, recipient ids.UUID, kind string) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = $2`, recipient, kind)
}

func taskOn(t *testing.T, e *integration.Env, subject string, assignee ids.UUID, recordType string, record ids.UUID) ids.UUID {
	t.Helper()
	to := ids.From[ids.UserKind](assignee)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual", AssigneeID: &to,
		Links: []activities.ActivityLinkInput{{EntityType: recordType, EntityID: record}},
	})
	if err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
	return ids.UUID(row.Id)
}

// handoverPipeline is the one pipeline a test's deals sit in.
type handoverPipeline struct {
	pipeline  ids.PipelineID
	open, won ids.StageID
}

func newHandoverPipeline(t *testing.T, e *integration.Env) handoverPipeline {
	t.Helper()
	pipeline, open, won := integration.DealFixture(t, e)
	return handoverPipeline{pipeline: pipeline, open: open, won: won}
}

func dealOn(t *testing.T, e *integration.Env, stages handoverPipeline, name string, owner, company ids.UUID) ids.UUID {
	t.Helper()
	companyID := ids.From[ids.CompanyKind](company)
	d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: stages.pipeline, StageID: stages.open,
		OwnerID: new(ids.From[ids.UserKind](owner)), CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("seeding deal %q: %v", name, err)
	}
	return ids.UUID(d.Id)
}

func ownerChangeAudits(t *testing.T, e *integration.Env, table string, id, from, to ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2
		    AND before->>'owner_id' = $3 AND after->>'owner_id' = $4`, table, id, from.String(), to.String())
}

// The owner accepts. The company, its open deal and the owner's open task on
// it move to the colleague who asked, each with its own audit row. A won deal
// and a third colleague's work stay where they are.
func TestAnAcceptedHandoverMovesTheCompanyAndTheOwnersOpenWorkOnIt(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "BAER Pharma", &e.Rep1)
	stages := newHandoverPipeline(t, e)
	openDeal := dealOn(t, e, stages, "BAER renewal", e.Rep1, company)
	wonDeal := dealOn(t, e, stages, "BAER pilot", e.Rep1, company)
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](wonDeal),
		deals.AdvanceDealInput{ToStageID: stages.won, WonWithoutContractReason: new("verbal")}); err != nil {
		t.Fatalf("winning the pilot: %v", err)
	}
	colleaguesDeal := dealOn(t, e, stages, "BAER upsell", e.Rep2, company)
	openTask := taskOn(t, e, "Call BAER back", e.Rep1, "company", company)
	colleaguesTask := taskOn(t, e, "Send BAER the deck", e.Rep2, "company", company)

	request, created := requestHandover(t, e, asker(e), "company", company)
	if !created || request.Transfer.FromID != e.Rep1 || request.Transfer.DeciderID != e.Rep1 {
		t.Fatalf("request = %+v (created %v), want a new request put to the owner", request, created)
	}
	summary := e.WsScalar(t, `SELECT summary FROM approval WHERE id = $1`, request.ApprovalID)
	if !strings.Contains(summary, "Open deals that move with it: 1. Open tasks: 1.") {
		t.Errorf("the card reads %q, want it to say one open deal and one open task move", summary)
	}
	if err := decideHandover(t, e, giver(e), request.ApprovalID, true); err != nil {
		t.Fatalf("the owner accepting: %v", err)
	}

	if got := ownerOf(t, e, "company", company); got != e.Rep3 {
		t.Errorf("the company is owned by %s, want the colleague who asked", got)
	}
	if got := ownerOf(t, e, "deal", openDeal); got != e.Rep3 {
		t.Errorf("the open deal is owned by %s, want it moved with the company", got)
	}
	if got := assigneeOf(t, e, openTask); got != e.Rep3 {
		t.Errorf("the open task is held by %s, want it moved with the company", got)
	}
	for name, stayed := range map[string]bool{
		"the won deal":       ownerOf(t, e, "deal", wonDeal) == e.Rep1,
		"a colleague's deal": ownerOf(t, e, "deal", colleaguesDeal) == e.Rep2,
		"a colleague's task": assigneeOf(t, e, colleaguesTask) == e.Rep2,
	} {
		if !stayed {
			t.Errorf("%s moved, want it left with its holder", name)
		}
	}
	for table, id := range map[string]ids.UUID{"company": company, "deal": openDeal} {
		if n := ownerChangeAudits(t, e, table, id, e.Rep1, e.Rep3); n != 1 {
			t.Errorf("%s owner-change audit rows = %d, want 1", table, n)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'company.updated' AND envelope->'entity'->>'id' = $1`, company.String()); n != 1 {
		t.Errorf("company.updated events = %d, want 1", n)
	}
	if n := noticesOf(t, e, e.Rep3, transferAcceptedKind); n != 1 {
		t.Errorf("acceptance notices to the colleague who asked = %d, want 1", n)
	}
	if consumed := e.WsScalar(t, `SELECT (consumed_at IS NOT NULL)::text FROM approval WHERE id = $1`, request.ApprovalID); consumed != "true" {
		t.Errorf("the accepted request is not consumed")
	}
}

// A contact moves with the open deals it is a stakeholder of and the open
// tasks on it that its owner holds.
func TestAnAcceptedHandoverMovesTheContactWithTheDealsItSitsOn(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Pat Example", &e.Rep1)
	company := e.SeedCompany(t, "Example AG", &e.Rep2)
	deal := dealOn(t, e, newHandoverPipeline(t, e), "Example rollout", e.Rep1, company)
	contactID, dealID := ids.From[ids.ContactKind](contact), ids.From[ids.DealKind](deal)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contactID, DealID: &dealID, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the contact on the deal: %v", err)
	}
	task := taskOn(t, e, "Ask Pat for the PO", e.Rep1, "contact", contact)

	request, _ := requestHandover(t, e, asker(e), "contact", contact)
	if err := decideHandover(t, e, giver(e), request.ApprovalID, true); err != nil {
		t.Fatalf("the owner accepting: %v", err)
	}
	if got := ownerOf(t, e, "contact", contact); got != e.Rep3 {
		t.Errorf("the contact is owned by %s, want the colleague who asked", got)
	}
	if got := ownerOf(t, e, "deal", deal); got != e.Rep3 {
		t.Errorf("the stakeholder deal is owned by %s, want it moved with the contact", got)
	}
	if got := assigneeOf(t, e, task); got != e.Rep3 {
		t.Errorf("the task on the contact is held by %s, want it moved", got)
	}
	if got := ownerOf(t, e, "company", company); got != e.Rep2 {
		t.Errorf("the deal's company moved to %s; only the contact was handed over", got)
	}
}

// Declined and run out: the record stays, and the colleague who asked is told
// each time. Declining leaves them free to ask again at once.
func TestADeclinedOrExpiredRequestLeavesTheRecordAndTellsTheAsker(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Kept GmbH", &e.Rep1)

	first, _ := requestHandover(t, e, asker(e), "company", company)
	if err := decideHandover(t, e, giver(e), first.ApprovalID, false); err != nil {
		t.Fatalf("the owner declining: %v", err)
	}
	if got := ownerOf(t, e, "company", company); got != e.Rep1 {
		t.Fatalf("a declined request moved the company to %s", got)
	}
	if n := noticesOf(t, e, e.Rep3, transferDeclined.kind); n != 1 {
		t.Errorf("decline notices = %d, want 1", n)
	}

	second, created := requestHandover(t, e, asker(e), "company", company)
	if !created || second.ApprovalID == first.ApprovalID {
		t.Fatalf("asking again after a decline answered %+v (created %v), want a new request", second, created)
	}
	if window := e.WsScalar(t, `SELECT (expires_at - created_at)::text FROM approval WHERE id = $1`, second.ApprovalID); window != "7 days" {
		t.Errorf("the request stays open for %s, want 7 days", window)
	}
	e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, second.ApprovalID)
	if _, err := expiringApprovalsService(e.Pool).ExpireDue(expiryCtx(e)); err != nil {
		t.Fatalf("the expiry sweep: %v", err)
	}
	if got := ownerOf(t, e, "company", company); got != e.Rep1 {
		t.Fatalf("an expired request moved the company to %s", got)
	}
	if n := noticesOf(t, e, e.Rep3, transferExpired.kind); n != 1 {
		t.Errorf("expiry notices = %d, want 1", n)
	}
}

// An administrator reassigns the company while a request waits: the request
// is withdrawn and its asker told. A request whose giver lost the record by a
// path that told nobody is refused on accept, and stays pending to decline.
func TestARecordThatChangedHandsClosesItsRequest(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Moved AG", &e.Rep1)
	request, _ := requestHandover(t, e, asker(e), "company", company)

	if _, err := newOwnershipTransfers(e.Pool).contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{
		OwnerID: new(ids.From[ids.UserKind](e.Rep2)),
	}); err != nil {
		t.Fatalf("the administrator reassigning: %v", err)
	}
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, request.ApprovalID); status != "expired" {
		t.Errorf("the request is %s after the company changed hands, want withdrawn", status)
	}
	if n := noticesOf(t, e, e.Rep3, transferWithdrawn.kind); n != 1 {
		t.Errorf("withdrawal notices = %d, want 1", n)
	}
	if n := transfersInInbox(t, e, giver(e)); n != 0 {
		t.Errorf("the previous owner still sees %d requests", n)
	}

	other := e.SeedCompany(t, "Quietly moved AG", &e.Rep1)
	late, _ := requestHandover(t, e, asker(e), "company", other)
	e.WsExec(t, `UPDATE company SET owner_id = $2 WHERE id = $1`, other, e.Rep2)
	if err := decideHandover(t, e, giver(e), late.ApprovalID, true); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a giver who can no longer change the record accepting → %v, want the card gone", err)
	}
	teamLead := handoverRepPerms()
	teamLead.RowScope = principal.RowScopeTeam
	teammate := e.As(e.Rep1, []ids.UUID{e.Team1}, teamLead)
	if p, ok := principal.Actor(teammate); ok {
		p.SeatType = principal.SeatFull
		teammate = principal.WithActor(teammate, p)
	}
	err := decideHandover(t, e, teammate, late.ApprovalID, true)
	if code := transferRefusalCode(err); !errors.Is(err, apperrors.ErrConflict) || code != "owner_changed" {
		t.Fatalf("a teammate who still may change the record accepting → %v (%q), want 409 owner_changed", err, code)
	}
	if got := ownerOf(t, e, "company", other); got != e.Rep2 {
		t.Errorf("a refused accept moved the company to %s", got)
	}
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, late.ApprovalID); status != "pending" {
		t.Errorf("a refused accept left the request %s, want it still pending to decline", status)
	}
}
