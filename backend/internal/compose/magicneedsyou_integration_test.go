// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The needs-you lane, bound the way the server binds it.
//
// The lane reaches the staged queue through an adapter of four lines — the
// status it filters on, the limit it forwards, the engine it asks and the
// binding itself — and every one of those is invisible to a suite that spells
// its own. So the service under test here is the one newMagicService returns,
// and the only thing this file supplies is the proposal.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/magic"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A DECISION WAITING REACHES THE RECEIPT THROUGH THE ENGINE THAT HOLDS IT.
//
// One engine stages the proposal and serves the lane, so the row drawn here is
// the row the inbox decides from rather than a second reading of the queue, and
// the row it returns is shaped by the engine's authority filter and its
// target-label freeze rather than by a fixture's idea of them.
func TestADecisionWaitingReachesTheReceiptThroughTheRealApprovalsEngine(t *testing.T) {
	e := integration.Setup(t)
	since := time.Now().Add(-time.Hour)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Weber GmbH — Phase 2", pipeline, open, &e.Rep1)

	queue := approvalsServiceWithEffects(e.Pool)
	change, hash, err := diffhash.Canonical(json.RawMessage(`{"summary":"Send revised pricing","seat_id":"` + e.Rep1.String() + `","promiser_seat_id":"` + e.Rep1.String() + `"}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.AgentCtx()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing staging actor")
	}
	actor.OnBehalfOf = e.Rep1
	ctx = principal.WithActor(ctx, actor)
	staged, err := queue.Stage(ctx, approvals.StageInput{
		Kind: "commitment_task", ProposedChange: change, DiffHash: hash,
		TargetType: "deal", TargetID: deal, Summary: "Send revised pricing",
	})
	if err != nil {
		t.Fatalf("staging the decision: %v", err)
	}

	perms := integration.RepPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	receipt, err := newMagicService(e.Pool, queue, time.Now).
		Read(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), &since, 20)
	if err != nil {
		t.Fatalf("reading the receipt: %v", err)
	}

	var line crmcontracts.MagicLine
	var found bool
	for _, candidate := range receipt.NeedsYou {
		if ids.UUID(candidate.Id) == staged.UUID {
			line, found = candidate, true
		}
	}
	if !found {
		t.Fatalf("a proposal the engine staged is absent from needs_you: %+v", receipt.NeedsYou)
	}
	if line.Summary.Key != "magic.action.approval_commitment_task_summary" {
		t.Errorf("summary key = %q, want the sentence naming the proposed task", line.Summary.Key)
	}
	if line.Summary.Values == nil || (*line.Summary.Values)["summary"] != "Send revised pricing" {
		t.Fatalf("proposed task is missing from the receipt: %+v", line.Summary.Values)
	}
	// The caption the engine froze at staging, which is what the approver was
	// shown; a lane resolving it itself would name whatever the record became.
	if line.Summary.Values == nil || (*line.Summary.Values)["target"] != "Weber GmbH — Phase 2" {
		t.Errorf("summary values = %v, want the target the engine recorded", line.Summary.Values)
	}
	if line.Entity == nil || ids.UUID(line.Entity.Id) != deal {
		t.Errorf("entity = %+v, want the deal the proposal is about", line.Entity)
	}
	// A decision waiting has nothing to undo, so the line carries no undo and
	// no refusal: the client draws "Decide" instead.
	if line.Undo != nil {
		t.Errorf("undo = %+v, want none on a decision nobody has made", *line.Undo)
	}
	if receipt.Totals.NeedsYou != len(receipt.NeedsYou) {
		t.Errorf("totals.needs_you says %d over %d drawn lines",
			receipt.Totals.NeedsYou, len(receipt.NeedsYou))
	}
}

func TestPersonalDecisionScopeAgreesAcrossHomeAndWorklist(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Example renewal", pipeline, open, &e.Rep1)
	queue := approvalsServiceWithEffects(e.Pool)
	ctx := e.AgentCtx()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing staging actor")
	}
	actor.OnBehalfOf = e.Rep1
	ctx = principal.WithActor(ctx, actor)
	requested, err := queue.Stage(ctx, approvals.StageInput{
		Kind: "commitment_task", ProposedChange: json.RawMessage(`{"summary":"Send pricing","seat_id":"` + e.Rep1.String() + `","promiser_seat_id":"` + e.Rep1.String() + `"}`),
		DiffHash: "personal-pricing", TargetType: "deal", TargetID: deal, Summary: "Send pricing",
	})
	if err != nil {
		t.Fatal(err)
	}
	background := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:background-review")
	background = onBehalfOf(background, e.Rep1)
	shared, err := queue.Stage(background, approvals.StageInput{
		Kind: "advance_deal", ProposedChange: json.RawMessage(`{"stage":"won"}`),
		DiffHash: "team-stage", TargetType: "deal", TargetID: deal, Summary: "Review the stage",
	})
	if err != nil {
		t.Fatal(err)
	}
	privateInput := activities.LogActivityInput{Kind: "meeting", Source: "manual", Body: new("I will send the private pricing."), OccurredAt: &now}
	privateInput.VisibleOnlyTo(e.Rep2)
	source, _, err := e.Activities.LogActivity(e.Admin(), privateInput)
	if err != nil {
		t.Fatal(err)
	}
	private, err := queue.Stage(onBehalfOf(background, e.Rep2), approvals.StageInput{
		Kind: "commitment_task", ProposedChange: json.RawMessage(`{"summary":"Send private pricing","seat_id":"` + e.Rep2.String() + `","promiser_seat_id":"` + e.Rep2.String() + `"}`),
		DiffHash: "private-pricing", TargetType: "activity", TargetID: ids.UUID(source.Id), Summary: "Send private pricing",
		Evidence: []approvals.Evidence{{SourceType: "activity", SourceID: ids.UUID(source.Id), Snippet: "I will send the private pricing."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, reader := range []ids.UUID{e.Rep1, e.Rep2, e.AdminUser} {
		t.Run(reader.String(), func(t *testing.T) {
			readCtx := e.As(reader, []ids.UUID{e.Team1}, integration.AdminPerms)
			homeResponse := httptest.NewRecorder()
			magic.NewHandlers(newMagicService(e.Pool, queue, func() time.Time { return now })).GetMagic(
				homeResponse, httptest.NewRequest(http.MethodGet, "/v1/magic", nil).WithContext(readCtx), crmcontracts.GetMagicParams{Since: &now, Limit: new(20)})
			var receipt crmcontracts.MagicReceipt
			if homeResponse.Code != http.StatusOK {
				t.Fatalf("Home HTTP %d: %s", homeResponse.Code, homeResponse.Body.String())
			}
			if err := json.Unmarshal(homeResponse.Body.Bytes(), &receipt); err != nil {
				t.Fatal(err)
			}
			home := map[string]bool{}
			for _, row := range receipt.NeedsYou {
				home[ids.UUID(row.Id).String()] = true
			}
			feed := newAttentionService(e.Pool, queue, func() time.Time { return now })
			work := readWorklistHTTP(readCtx, t, feed, crmcontracts.GetWorklistParams{})
			mine := map[string]bool{}
			for _, row := range work.Queue {
				mine[row.Id] = true
			}
			want := reader == e.Rep1
			if home[requested.String()] != want || mine[requested.String()] != want {
				t.Fatalf("requested decision: home=%v worklist=%v want=%v", home[requested.String()], mine[requested.String()], want)
			}
			privateWanted := reader == e.Rep2
			if home[private.String()] != privateWanted || mine[private.String()] != privateWanted {
				t.Fatalf("private decision: home=%v worklist=%v want=%v", home[private.String()], mine[private.String()], privateWanted)
			}
			if receipt.Totals.NeedsYou != len(receipt.NeedsYou) {
				t.Fatalf("Home count %d differs from %d readable decisions", receipt.Totals.NeedsYou, len(receipt.NeedsYou))
			}
			if home[shared.String()] != want || mine[shared.String()] != want {
				t.Fatalf("a background review staged for the deal's owner: home=%v worklist=%v want=%v",
					home[shared.String()], mine[shared.String()], want)
			}
			for _, scope := range []crmcontracts.GetWorklistParamsScope{"unassigned", "all", "team"} {
				page := readWorklistHTTP(readCtx, t, feed, crmcontracts.GetWorklistParams{Scope: &scope})
				listed := map[string]bool{}
				for _, row := range page.Queue {
					if row.Source == "approval" {
						listed[row.Id] = true
					}
				}
				sharedListed := scope == "all" || (scope == "team" && reader != e.AdminUser)
				if listed[shared.String()] != sharedListed || listed[requested.String()] != (want && scope != "unassigned") || listed[private.String()] != (privateWanted && scope != "unassigned") {
					t.Fatalf("scope %s has incorrect decision ownership or privacy: %v", scope, listed)
				}
				for _, count := range page.Counts {
					if count.Category == "decisions" && (count.Considered != len(listed) || count.Shown != len(listed) || count.MoreAvailable) {
						t.Fatalf("scope %s count %+v disagrees with %d reachable decisions", scope, count, len(listed))
					}
				}
			}
		})
	}
}

func readWorklistHTTP(ctx context.Context, t *testing.T, service *attention.Service, params crmcontracts.GetWorklistParams) crmcontracts.Worklist {
	t.Helper()
	if params.Limit == nil {
		params.Limit = new(100)
	}
	response := httptest.NewRecorder()
	attention.NewHandlers(service).GetWorklist(response, httptest.NewRequest(http.MethodGet, "/v1/worklist", nil).WithContext(ctx), params)
	if response.Code != http.StatusOK {
		t.Fatalf("Worklist HTTP %d: %s", response.Code, response.Body.String())
	}
	var work crmcontracts.Worklist
	if err := json.Unmarshal(response.Body.Bytes(), &work); err != nil {
		t.Fatal(err)
	}
	return work
}
