// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A proposal that went out on an open deal asks the deal's owner whether to
// move it, and only the owner's accept moves it. The tests drive the trigger
// and the reading with the events and rows the real writers produce. They
// decide through the deciding approvals service.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const proposalOurDomain = "acme-sales.example"

type proposalEnv struct {
	*integration.Env
	trigger  *StageEvidenceTrigger
	pipeline ids.PipelineID
	stages   []ids.StageID
	deal     ids.DealID
}

// newProposalEnv seeds the ladder Lead, Qualified, Discovery, Scoping,
// Proposal Sent, Negotiation and a deal on Qualified, the second stage.
func newProposalEnv(t *testing.T) *proposalEnv {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, err := e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatalf("seeding a pipeline: %v", err)
	}
	p := &proposalEnv{Env: e, pipeline: ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))}
	for i, name := range []string{"Lead", "Qualified", "Discovery", "Scoping", "Proposal Sent", "Negotiation"} {
		stage, err := e.Deals.CreateStage(ctx, deals.CreateStageInput{
			PipelineID: p.pipeline, Name: name, Position: i, Semantic: string(deals.SemanticOpen),
		})
		if err != nil {
			t.Fatalf("seeding stage %s: %v", name, err)
		}
		p.stages = append(p.stages, ids.From[ids.StageKind](ids.UUID(stage.Id)))
	}
	p.deal = ids.From[ids.DealKind](e.SeedDeal(t, "Hanoi depot", p.pipeline, p.stages[1], &e.AdminUser))
	p.trigger = NewStageEvidenceTrigger(e.Pool, e.Deals, staticDomains{proposalOurDomain}, nil, slog.Default())
	return p
}

// sendOffer writes a real offer on the deal and sends it, and answers the
// offer.sent envelope its send put on the deal stream.
func (p *proposalEnv) sendOffer(t *testing.T) kevents.Envelope {
	t.Helper()
	description, price, tax := "Rollout", int64(4_000_000), "0.00"
	desk := p.As(p.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"deal_desk"},
		Objects: map[string]principal.ObjectGrant{
			"deal": {Read: true, Update: true}, "offer": {Create: true, Read: true, Update: true},
			"installation_settings": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	})
	created, err := p.Deals.CreateOffer(desk, p.deal, deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &tax}},
	})
	if err != nil {
		t.Fatalf("creating the offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := p.Deals.SendOffer(desk, offer, nil); err != nil {
		t.Fatalf("sending the offer: %v", err)
	}
	return evidenceEnvelope("offer.sent", "offer", offer.UUID,
		crmcontracts.PublicEventOfferSent{OfferId: created.Id, DealId: created.DealId, FxRateToBase: "1"})
}

func (p *proposalEnv) handle(t *testing.T, env kevents.Envelope) {
	t.Helper()
	if err := p.trigger.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling %s: %v", env.Type, err)
	}
}

type proposalCard struct {
	id     ids.ApprovalID
	move   deals.ProposalStageMove
	status string
	decide ids.UUID
}

// cards answers every question of this kind the deal ever had.
func (p *proposalEnv) cards(t *testing.T) []proposalCard {
	t.Helper()
	var out []proposalCard
	if err := database.WithWorkspaceTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT id, proposed_change, status, requested_decider_id FROM approval
			 WHERE kind = $1 AND target_entity_id = $2 ORDER BY created_at, id`, deals.ProposalStageKind, p.deal)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (proposalCard, error) {
			var c proposalCard
			var raw []byte
			if err := row.Scan(&c.id, &raw, &c.status, &c.decide); err != nil {
				return c, err
			}
			return c, json.Unmarshal(raw, &c.move)
		})
		return err
	}); err != nil {
		t.Fatalf("reading the questions: %v", err)
	}
	return out
}

func (p *proposalEnv) pending(t *testing.T) []proposalCard {
	t.Helper()
	var out []proposalCard
	for _, c := range p.cards(t) {
		if c.status == "pending" {
			out = append(out, c)
		}
	}
	return out
}

func (p *proposalEnv) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	return p.WsScalar(t, query, args...)
}

func TestASentOfferAsksToMoveTheDealAndAcceptingMovesIt(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.sendOffer(t))

	open := p.pending(t)
	if len(open) != 1 {
		t.Fatalf("a sent offer raised %d questions, want 1", len(open))
	}
	card := open[0]
	if card.move.ToStageID != p.stages[4].UUID || card.move.MatchedBy != deals.ProposalMatchedByStageName {
		t.Fatalf("the question names %+v, want Proposal Sent by its name", card.move)
	}
	if len(card.move.SkippedStageIDs) != 2 || card.move.SkippedStageIDs[0] != p.stages[2].UUID || card.move.SkippedStageIDs[1] != p.stages[3].UUID {
		t.Fatalf("skipped stages = %v, want Discovery and Scoping", card.move.SkippedStageNames)
	}
	if card.decide != p.AdminUser {
		t.Fatalf("the question is put to %s, want the deal's owner", card.decide)
	}
	if stage := p.scalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, p.deal); stage != p.stages[1].String() {
		t.Fatalf("raising the question moved the deal to %s", stage)
	}

	svc := NewApprovalsServiceWithEffects(p.Pool)
	if _, err := svc.Decide(p.Admin(), card.id, true, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if stage := p.scalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, p.deal); stage != p.stages[4].String() {
		t.Fatalf("the deal is on %s after the accept, want Proposal Sent", stage)
	}
	if n := p.scalar(t, `SELECT count(*) FROM deal_stage_history WHERE deal_id = $1 AND to_stage_id = $2 AND approval_id = $3`,
		p.deal, p.stages[4], card.id); n != "1" {
		t.Fatalf("%s stage history rows carry the approval, want 1", n)
	}
	if n := p.scalar(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'advance_stage' AND evidence->>$2 = $3`,
		p.deal, deals.StageApprovalEvidenceKey, card.id.String()); n != "1" {
		t.Fatalf("%s advance_stage audit rows name the approval, want 1", n)
	}
	if n := p.scalar(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'deal.stage_changed' AND envelope->'entity'->>'id' = $1`,
		p.deal.String()); n != "1" {
		t.Fatalf("%s deal.stage_changed events, want 1", n)
	}
}

func TestADeclinedQuestionIsNotAskedAgainForTheSameOfferButIsForANewOne(t *testing.T) {
	p := newProposalEnv(t)
	first := p.sendOffer(t)
	p.handle(t, first)
	svc := NewApprovalsServiceWithEffects(p.Pool)
	if _, err := svc.Decide(p.Admin(), p.pending(t)[0].id, false, nil); err != nil {
		t.Fatalf("declining: %v", err)
	}
	p.handle(t, first)
	if n := len(p.pending(t)); n != 0 {
		t.Fatalf("the same offer asked again after Not now: %d open", n)
	}
	p.handle(t, p.sendOffer(t))
	if n := len(p.pending(t)); n != 1 {
		t.Fatalf("a new offer raised %d questions, want 1", n)
	}
}

func TestNoQuestionWhereThereIsNothingToMove(t *testing.T) {
	cases := map[string]func(*testing.T, *proposalEnv){
		"legal hold": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, p.deal)
		},
		"already past the proposal stage": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET stage_id = $2 WHERE id = $1`, p.deal, p.stages[5])
		},
		"already at the proposal stage": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET stage_id = $2 WHERE id = $1`, p.deal, p.stages[4])
		},
		"no owner": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, p.deal)
		},
		"closed deal": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET status = 'lost', closed_at = now(), lost_reason = 'gone' WHERE id = $1`, p.deal)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProposalEnv(t)
			envelope := p.sendOffer(t)
			setup(t, p)
			p.handle(t, envelope)
			if n := len(p.cards(t)); n != 0 {
				t.Fatalf("%d questions raised, want none", n)
			}
		})
	}
}

func TestMovingTheDealPastTheStageWithdrawsTheQuestion(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.sendOffer(t))
	if _, err := p.Deals.AdvanceDeal(p.Admin(), p.deal, deals.AdvanceDealInput{ToStageID: p.stages[5]}); err != nil {
		t.Fatalf("moving the deal by hand: %v", err)
	}
	p.handle(t, evidenceEnvelope("deal.stage_changed", "deal", p.deal.UUID, crmcontracts.PublicEventDealStageChanged{}))
	if n := len(p.pending(t)); n != 0 {
		t.Fatalf("%d questions still open after the deal moved past the stage", n)
	}
}

func TestAnUnansweredQuestionExpiresAfterFourteenDays(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.sendOffer(t))
	days := p.scalar(t, `SELECT round(extract(epoch FROM expires_at - created_at) / 86400)::text FROM approval WHERE kind = $1 AND target_entity_id = $2`,
		deals.ProposalStageKind, p.deal)
	if days != "14" {
		t.Fatalf("the question stays open %s days, want 14", days)
	}
}

// ---- the mail arm ----

// outboundMail writes our own mail on the deal, with one attachment per name
// given. It records the mail as capture does: our seat as sender, the buyer
// as recipient, the files on the activity.
func (p *proposalEnv) outboundMail(t *testing.T, body string, files ...string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, direction, subject, body, occurred_at, source, captured_by)
			VALUES ('email', 'outbound', 'Your depot', $1, $2, 'manual', 'human:test') RETURNING id`,
			body, time.Now().Add(-time.Hour)).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'from')`, id, p.AdminUser); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, 'ines@customer.example', 'to')`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, p.deal); err != nil {
			return err
		}
		for i, name := range files {
			if _, err := tx.Exec(ctx, `
				INSERT INTO attachment (entity_type, entity_id, filename, content_type, storage_key, source, captured_by, category, activity_id)
				VALUES ('activity', $1, $2, 'application/pdf', $3, 'manual', 'human:test', 'email_attachment', $1)`,
				id, name, name+string(rune('a'+i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding our mail: %v", err)
	}
	return id
}

func (p *proposalEnv) captured(t *testing.T, activity ids.UUID) kevents.Envelope {
	t.Helper()
	return evidenceEnvelope("activity.captured", "activity", activity, crmcontracts.PublicEventActivityCaptured{Kind: "email"})
}

func TestOurMailWithAnAttachmentNamedLikeAProposalAsksWithoutAModel(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.captured(t, p.outboundMail(t, "See attached.", "Angebot_Depot_2026.pdf")))
	open := p.pending(t)
	if len(open) != 1 || open[0].move.SourceType != deals.ProposalSourceActivity {
		t.Fatalf("a mail with Angebot_Depot_2026.pdf raised %+v, want one question from the activity", open)
	}
}

func TestAnAttachmentThatIsNotNamedLikeAProposalAsksNothing(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.captured(t, p.outboundMail(t, "See attached.", "Rechnung_2026.pdf", "photo.png")))
	if n := len(p.cards(t)); n != 0 {
		t.Fatalf("%d questions raised for an invoice and a photo", n)
	}
}

func TestTheOfferAndTheMailCarryingItAskOnce(t *testing.T) {
	p := newProposalEnv(t)
	p.handle(t, p.sendOffer(t))
	p.handle(t, p.captured(t, p.outboundMail(t, "Offer attached.", "Offer_Depot.pdf")))
	if n := len(p.pending(t)); n != 1 {
		t.Fatalf("%d open questions for one proposal, want 1", n)
	}
}

// claimReply is a model that answers the reserved question as given.
type claimReply struct {
	met, quote string
	confidence float64
}

func (c claimReply) Complete(context.Context, model.Request) (model.Response, error) {
	body, err := json.Marshal(map[string]any{"claims": []map[string]any{{
		"criterion_key": deals.ProposalSentCriterionKey, "source_id": "s1", "source_lines": []int{1},
		"quote": c.quote, "met": c.met, "commitment": "agreed", "confidence": c.confidence,
	}}})
	return model.Response{Text: string(body)}, err
}

func (p *proposalEnv) read(t *testing.T, brain completer, activity ids.UUID) {
	t.Helper()
	reader := NewStageEvidenceReader(p.Pool, p.Deals, staticDomains{proposalOurDomain}, brain, time.Now, slog.Default())
	if _, err := reader.Read(p.Admin(), p.deal, activity); err != nil {
		t.Fatalf("reading the mail: %v", err)
	}
}

func TestTheReadingRaisesTheQuestionOnlyForAConfidentYes(t *testing.T) {
	const body = "Please find our quote attached."
	cases := map[string]struct {
		reply claimReply
		cards int
	}{
		"confident yes":    {claimReply{"true", body, 0.9}, 1},
		"below the floor":  {claimReply{"true", body, 0.4}, 0},
		"the text says no": {claimReply{"false", body, 0.9}, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProposalEnv(t)
			p.read(t, tc.reply, p.outboundMail(t, body, "scan0001.pdf"))
			if n := len(p.pending(t)); n != tc.cards {
				t.Fatalf("%d questions, want %d", n, tc.cards)
			}
			if n := p.scalar(t, `SELECT count(*) FROM deal_stage_evidence WHERE deal_id = $1`, p.deal); n != "0" {
				t.Fatalf("the proposal answer reached the evidence ledger: %s rows", n)
			}
		})
	}
}

func TestTheReadingDoesNotAskAboutAMailWithNoAttachment(t *testing.T) {
	p := newProposalEnv(t)
	p.read(t, claimReply{"true", "Please find our quote attached.", 0.9}, p.outboundMail(t, "Please find our quote attached."))
	if n := len(p.cards(t)); n != 0 {
		t.Fatalf("a mail with no file raised %d questions", n)
	}
}

func TestAnAcceptAfterTheDealMovedOrWasHeldWritesNothing(t *testing.T) {
	cases := map[string]func(*testing.T, *proposalEnv){
		"the deal reached the stage by hand": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET stage_id = $2 WHERE id = $1`, p.deal, p.stages[4])
		},
		"the deal went on legal hold": func(t *testing.T, p *proposalEnv) {
			p.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, p.deal)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProposalEnv(t)
			p.handle(t, p.sendOffer(t))
			card := p.pending(t)[0]
			change(t, p)
			before := p.scalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, p.deal)
			svc := NewApprovalsServiceWithEffects(p.Pool)
			if _, err := svc.Decide(p.Admin(), card.id, true, nil); err == nil {
				t.Fatal("the accept went through although the question no longer applies")
			}
			if after := p.scalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, p.deal); after != before {
				t.Fatalf("the deal moved from %s to %s", before, after)
			}
			if n := p.scalar(t, `SELECT count(*) FROM deal_stage_history WHERE approval_id = $1`, card.id); n != "0" {
				t.Fatalf("%s history rows name the refused approval", n)
			}
		})
	}
}
