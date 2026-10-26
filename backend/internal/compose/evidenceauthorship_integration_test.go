// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The evidence authorship pass judges the author of stored stage evidence
// again after an own domain is added or removed. The domain changes go
// through the admin list the settings page writes.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type authorshipFixture struct {
	*integration.Env
	t      *testing.T
	dealID ids.DealID
	crit   map[string]ids.ExitCriterionID
	own    *capture.OwnDomainStore
}

func newAuthorshipFixture(t *testing.T) *authorshipFixture {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatal(err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Qualified", Position: 0, Semantic: string(deals.SemanticOpen),
	})
	if err != nil {
		t.Fatal(err)
	}
	stageID := ids.From[ids.StageKind](ids.UUID(stage.Id))
	f := &authorshipFixture{Env: e, t: t, crit: map[string]ids.ExitCriterionID{}, own: capture.NewOwnDomainStore(e.DB())}
	for key, kind := range map[string]deals.CriterionKind{
		"budget": deals.CriterionBuyerConfirmed, "terms": deals.CriterionTermsAccepted, "notes": deals.CriterionCustom,
	} {
		c, err := e.Deals.CreateStageExitCriterion(ctx, deals.CreateCriterionInput{StageID: stageID, Key: key, Label: key, Kind: string(kind)})
		if err != nil {
			t.Fatal(err)
		}
		f.crit[key] = ids.From[ids.ExitCriterionKind](ids.UUID(c.Id))
	}
	f.dealID = ids.From[ids.DealKind](e.SeedDeal(t, "Rollout", pipelineID, stageID, &e.AdminUser))
	return f
}

// inboundMail writes mail from the sender, linked to the deal, as capture
// stores it: an inbound row with the sender as its From participant.
func (f *authorshipFixture) inboundMail(sender string) ids.UUID {
	f.t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, subject, direction, occurred_at, source, captured_by)
			VALUES ('email', 'Budget', 'inbound', $1, 'gmail:seed', 'connector:gmail') RETURNING id`,
			clocktest.Now(f.t).Add(-time.Hour)).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'from')`, id, sender); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, f.dealID.UUID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// claim records a model reading of the mail through the ledger's writer,
// with the side the shared judgement gives today.
func (f *authorshipFixture) claim(mail ids.UUID, criterion string) ids.UUID {
	f.t.Helper()
	confidence := 0.9
	ev, err := f.Deals.RecordStageEvidence(f.Admin(), deals.EvidenceInput{
		DealID: f.dealID, CriterionID: f.crit[criterion], SourceType: deals.SourceActivity, SourceID: mail,
		AuthorSide: f.judged(mail), Commitment: deals.CommitmentAgreed, Met: true, Confidence: &confidence,
		ObservedAt: clocktest.Now(f.t).Add(-time.Hour), ExtractedBy: stageEvidenceExtractedBy,
	})
	if err != nil {
		f.t.Fatalf("recording evidence for %s: %v", criterion, err)
	}
	return ids.UUID(ev.Id)
}

// judged is the side the evidence writers would give the mail now.
func (f *authorshipFixture) judged(mail ids.UUID) deals.AuthorSide {
	f.t.Helper()
	var side deals.AuthorSide
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		facts, err := deals.ReadActivityAuthorship(f.Admin(), tx, mail, ownDomainReader{store: f.own})
		side = facts.AuthorSide
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return side
}

func (f *authorshipFixture) stored(evidence ids.UUID) string {
	f.t.Helper()
	return f.WsScalar(f.t, `SELECT author_side FROM deal_stage_evidence WHERE id = $1`, evidence)
}

func (f *authorshipFixture) runPass() {
	f.t.Helper()
	worker := newEvidenceAuthorshipWorker(f.Pool, slog.New(slog.DiscardHandler))
	if err := worker.Work(f.Admin(), &river.Job[EvidenceAuthorshipArgs]{}); err != nil {
		f.t.Fatalf("running the authorship pass: %v", err)
	}
}

func (f *authorshipFixture) corrections(evidence ids.UUID) int {
	f.t.Helper()
	return f.WsCount(f.t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_stage_evidence' AND entity_id = $1
		AND action = 'update' AND actor_type = 'system' AND evidence->>'rejudged_against' = 'own_domains'`, evidence)
}

func TestStageEvidenceFollowsAnOwnDomainAddedAndRemoved(t *testing.T) {
	f := newAuthorshipFixture(t)
	fromBuyer := f.inboundMail("cfo@buyer-corp.test")
	fromOther := f.inboundMail("ops@other-customer.test")
	budget := f.claim(fromBuyer, "budget")
	other := f.claim(fromOther, "notes")
	// A member struck this claim out. Their decision stands whatever the
	// domains say later.
	refuted := f.claim(fromBuyer, "terms")
	if _, err := f.Deals.RefuteStageEvidence(f.Admin(), f.dealID, refuted); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]ids.UUID{"budget": budget, "other": other, "refuted": refuted} {
		if got := f.stored(id); got != string(deals.AuthorBuyer) {
			t.Fatalf("before any change the %s claim is %s, want buyer", name, got)
		}
	}
	dealEvents := func() int {
		return f.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'deal.updated'
			AND envelope->'entity'->>'id' = $1`, f.dealID.UUID.String())
	}
	eventsBefore := dealEvents()

	// The buyer's domain turns out to be ours.
	if _, err := f.own.Add(f.Admin(), "Buyer-Corp.test"); err != nil {
		t.Fatal(err)
	}
	f.runPass()
	if got := f.stored(budget); got != string(f.judged(fromBuyer)) || got != string(deals.AuthorSeller) {
		t.Errorf("after the domain was added the budget claim is %s, the writers judge %s, want seller", got, f.judged(fromBuyer))
	}
	if deals.SettlesBuyerMilestone(deals.CriterionBuyerConfirmed, deals.AuthorSide(f.stored(budget))) {
		t.Errorf("our own mail still settles the buyer's budget confirmation")
	}
	if got := f.stored(other); got != string(deals.AuthorBuyer) {
		t.Errorf("a claim from another domain changed to %s", got)
	}
	if got := f.stored(refuted); got != string(deals.AuthorBuyer) {
		t.Errorf("the refuted claim changed to %s", got)
	}
	if got := f.WsCount(t, `SELECT count(*) FROM deal_stage_evidence WHERE id = $1 AND refuted_at IS NOT NULL`, refuted); got != 1 {
		t.Errorf("the member's refutation was lifted")
	}
	if got := f.corrections(budget); got != 1 {
		t.Errorf("budget correction audit rows = %d, want 1", got)
	}
	if got := dealEvents(); got != eventsBefore+1 {
		t.Errorf("deal.updated events %d, want %d: one for the corrected claim", got, eventsBefore+1)
	}

	// A repeated pass finds nothing left to change.
	f.runPass()
	if got := f.corrections(budget); got != 1 {
		t.Errorf("a repeated pass wrote again: %d correction rows", got)
	}

	// The domain was a mistake after all.
	if err := f.own.Remove(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	f.runPass()
	if got := f.stored(budget); got != string(f.judged(fromBuyer)) || got != string(deals.AuthorBuyer) {
		t.Errorf("after the domain was removed the budget claim is %s, the writers judge %s, want buyer", got, f.judged(fromBuyer))
	}
	if got := f.corrections(budget); got != 2 {
		t.Errorf("budget correction audit rows = %d, want 2", got)
	}
	if got := f.corrections(other) + f.corrections(refuted); got != 0 {
		t.Errorf("unaffected claims carry %d correction rows", got)
	}
}
