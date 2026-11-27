// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package proposalstage_test

// A proposal that went out on an open deal asks the deal's owner whether to
// move it, and only the owner's accept moves it. The tests drive the trigger
// and the reading with the events and rows the real writers produce. They
// decide through the deciding approvals service.

import (
	"context"
	"encoding/json"
	"log/slog"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/proposalstage"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// staticDomains is the own-domain seam, answering a fixed list.
type staticDomains []string

func (d staticDomains) Domains(context.Context, pgx.Tx) ([]string, error) {
	return []string(d), nil
}

func (d staticDomains) MeetingParties(context.Context, pgx.Tx) (owndomain.Parties, error) {
	own := owndomain.New(d)
	return owndomain.Parties{Trusted: own, Possible: own}, nil
}

//craft:ignore naked-any the payload goes straight to json.Marshal, and each test passes a different generated event struct
func evidenceEnvelope(t testing.TB, eventType, entityType string, entity ids.UUID, payload any) kevents.Envelope {
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return kevents.Envelope{
		EventID: ids.NewV7(), Type: eventType, Version: 1,
		OccurredAt: clocktest.Now(t).UTC(),
		Entity:     kevents.EntityRef{Type: entityType, ID: entity},
		Payload:    raw,
		Trace:      kevents.Trace{CorrelationID: ids.NewV7()},
	}
}

const proposalOurDomain = "acme-sales.example"

type proposalEnv struct {
	*integration.Env
	trigger  *compose.StageEvidenceTrigger
	suggest  *proposalstage.Suggester
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
	p.trigger = compose.NewStageEvidenceTrigger(e.Pool, e.Deals, staticDomains{proposalOurDomain}, nil, slog.Default())
	p.suggest = proposalstage.New(e.DB(), compose.NewApprovalsService(e.DB()))
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
	return evidenceEnvelope(t, "offer.sent", "offer", offer.UUID,
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
	if err := database.WithTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
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

	svc := compose.NewApprovalsServiceWithEffects(p.Pool)
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
	svc := compose.NewApprovalsServiceWithEffects(p.Pool)
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
	stale := evidenceEnvelope(t, "deal.stage_changed", "deal", p.deal.UUID, crmcontracts.PublicEventDealStageChanged{})
	stale.OccurredAt = clocktest.Now(t).Add(-8 * 24 * time.Hour)
	p.handle(t, stale)
	if n := len(p.pending(t)); n != 0 {
		t.Fatalf("%d questions still open after an 8-day-old stage change; retirement must not be dropped as backlog", n)
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
	if err := database.WithTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, direction, subject, body, occurred_at, source, captured_by)
			VALUES ('email', 'outbound', 'Your depot', $1, $2, 'manual', 'human:test') RETURNING id`,
			body, clocktest.Now(t).Add(-time.Hour)).Scan(&id); err != nil {
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
	return evidenceEnvelope(t, "activity.captured", "activity", activity, crmcontracts.PublicEventActivityCaptured{Kind: "email"})
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
	key        string
}

func (c claimReply) Complete(context.Context, model.Request) (model.Response, error) {
	body, err := json.Marshal(map[string]any{"claims": []map[string]any{{
		"criterion_key": cmpOr(c.key, deals.ProposalSentCriterionKey), "source_id": "s1", "source_lines": []int{1},
		"quote": c.quote, "met": c.met, "commitment": "agreed", "confidence": c.confidence,
	}}})
	return model.Response{Text: string(body)}, err
}

func cmpOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (p *proposalEnv) read(t *testing.T, brain interface {
	Complete(context.Context, model.Request) (model.Response, error)
}, activity ids.UUID,
) {
	t.Helper()
	reader := compose.NewStageEvidenceReader(p.Pool, p.Deals, staticDomains{proposalOurDomain}, brain, func() time.Time { return clocktest.Now(t) }, slog.Default())
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
		"confident yes":    {claimReply{met: "true", quote: body, confidence: 0.9}, 1},
		"below the floor":  {claimReply{met: "true", quote: body, confidence: 0.4}, 0},
		"the text says no": {claimReply{met: "false", quote: body, confidence: 0.9}, 0},
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
	p.read(t, claimReply{met: "true", quote: "Please find our quote attached.", confidence: 0.9}, p.outboundMail(t, "Please find our quote attached."))
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
			svc := compose.NewApprovalsServiceWithEffects(p.Pool)
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

func TestOnlyTheDealsOwnerSeesAndDecidesTheQuestionAndAReassignmentHandsItOn(t *testing.T) {
	p := newProposalEnv(t)
	p.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, p.deal, p.Rep1)
	p.handle(t, p.sendOffer(t))
	card := p.pending(t)[0]
	if card.decide != p.Rep1 {
		t.Fatalf("the question is put to %s, want the owner Rep1", card.decide)
	}
	svc := compose.NewApprovalsServiceWithEffects(p.Pool)
	manager := p.As(p.AdminUser, nil, integration.AdminPerms)
	if _, err := svc.Get(manager, card.id); err == nil {
		t.Fatal("another manager can read the owner's question")
	}
	if _, err := svc.Decide(manager, card.id, true, nil); err == nil {
		t.Fatal("another manager decided the owner's question")
	}
	newOwner := ids.From[ids.UserKind](p.Rep2)
	if _, err := p.Deals.UpdateDeal(p.Admin(), p.deal, deals.UpdateDealInput{OwnerID: &newOwner}); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}
	if _, err := svc.Decide(p.As(p.Rep1, nil, integration.AdminPerms), card.id, true, nil); err == nil {
		t.Fatal("the previous owner decided the question after the reassignment")
	}
	if _, err := svc.Decide(p.As(p.Rep2, nil, integration.AdminPerms), card.id, true, nil); err != nil {
		t.Fatalf("the new owner could not decide: %v", err)
	}
}

func TestAUserCriterionUnderTheProposalKeysOldSpellingIsNotTheProposalQuestion(t *testing.T) {
	p := newProposalEnv(t)
	if _, err := p.Deals.CreateStageExitCriterion(p.Admin(), deals.CreateCriterionInput{
		StageID: p.stages[1], Key: "margince_proposal_sent", Label: "Quote out", Kind: string(deals.CriterionCustom),
	}); err != nil {
		t.Fatalf("seeding the look-alike criterion: %v", err)
	}
	p.read(t, claimReply{met: "true", quote: "Please find our quote attached.", confidence: 0.9, key: "margince_proposal_sent"},
		p.outboundMail(t, "Please find our quote attached.", "scan0001.pdf"))
	if n := len(p.pending(t)); n != 0 {
		t.Fatalf("a claim on a user criterion raised %d proposal questions", n)
	}
}

func TestAnAcceptWritesNothingWhenTheAnnouncingFileIsGone(t *testing.T) {
	cases := map[string]string{
		"the file was removed":           `UPDATE attachment SET archived_at = now() WHERE activity_id = $1`,
		"the file's bytes were withheld": `UPDATE attachment SET bytes_withheld = true WHERE activity_id = $1`,
		"the file was renamed":           `UPDATE attachment SET filename = 'photo.png', content_type = 'image/png' WHERE activity_id = $1`,
		"the mail moved to another deal": `DELETE FROM activity_link WHERE activity_id = $1`,
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProposalEnv(t)
			mail := p.outboundMail(t, "See attached.", "Angebot_Depot.pdf")
			p.handle(t, p.captured(t, mail))
			card := p.pending(t)[0]
			if card.move.SourceAttachmentID == nil || !card.move.SourceFileNamed {
				t.Fatalf("the question does not carry its file: %+v", card.move)
			}
			p.WsExec(t, change, mail)
			svc := compose.NewApprovalsServiceWithEffects(p.Pool)
			if _, err := svc.Decide(p.Admin(), card.id, true, nil); err == nil {
				t.Fatal("the accept went through")
			}
			if stage := p.scalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, p.deal); stage != p.stages[1].String() {
				t.Fatalf("the deal moved to %s", stage)
			}
		})
	}
}

func TestSimultaneousSourcesRaiseOneQuestion(t *testing.T) {
	p := newProposalEnv(t)
	mail := p.outboundMail(t, "See attached.", "Angebot_Depot.pdf")
	sources := []proposalstage.Source{
		{Type: deals.ProposalSourceOffer, ID: ids.NewV7(), Snippet: "Offer sent"},
		{Type: deals.ProposalSourceOffer, ID: ids.NewV7(), Snippet: "Offer sent"},
		{Type: deals.ProposalSourceActivity, ID: mail, Snippet: "Attached file"},
	}
	start := make(chan struct{})
	errs := make(chan error, len(sources))
	for _, src := range sources {
		go func() {
			<-start
			errs <- p.suggest.Suggest(context.Background(), p.deal, src)
		}()
	}
	close(start)
	for range sources {
		if err := <-errs; err != nil {
			t.Fatalf("suggesting: %v", err)
		}
	}
	if n := len(p.pending(t)); n != 1 {
		t.Fatalf("%d open questions from three simultaneous sources, want 1", n)
	}
}

func TestAStageMoveInFlightDecidesBeforeAQuestionIsRaised(t *testing.T) {
	p := newProposalEnv(t)
	moving := make(chan struct{})
	release := make(chan struct{})
	moved := make(chan error, 1)
	go func() {
		moved <- database.WithTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), `UPDATE deal SET stage_id = $2 WHERE id = $1`, p.deal, p.stages[4]); err != nil {
				return err
			}
			close(moving)
			<-release
			return nil
		})
	}()
	<-moving
	suggested := make(chan error, 1)
	go func() {
		suggested <- p.suggest.Suggest(context.Background(), p.deal, proposalstage.Source{Type: deals.ProposalSourceOffer, ID: ids.NewV7(), Snippet: "Offer sent"})
	}()
	p.waitForLock(t, "%FOR SHARE OF d%")
	close(release)
	if err := <-moved; err != nil {
		t.Fatalf("the stage move: %v", err)
	}
	if err := <-suggested; err != nil {
		t.Fatalf("suggesting: %v", err)
	}
	if n := len(p.cards(t)); n != 0 {
		t.Fatalf("%d questions raised for a deal that had just reached the stage", n)
	}
}

func (p *proposalEnv) waitForLock(t *testing.T, pattern string) {
	t.Helper()
	for tries := 0; p.scalar(t, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE $1`, pattern) == "0"; tries++ {
		if tries > 20000 {
			t.Fatalf("nothing waited on a lock like %q", pattern)
		}
		runtime.Gosched()
	}
}

func TestASuggestionWaitsForTheDealsLockBeforeScanningForOpenQuestions(t *testing.T) {
	p := newProposalEnv(t)
	holding := make(chan struct{})
	release := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		held <- database.WithTx(p.Admin(), p.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(context.Background(), proposalstage.LockSQL, p.deal); err != nil {
				return err
			}
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	suggested := make(chan error, 1)
	go func() {
		suggested <- p.suggest.Suggest(context.Background(), p.deal, proposalstage.Source{Type: deals.ProposalSourceOffer, ID: ids.NewV7(), Snippet: "Offer sent"})
	}()
	p.waitForLock(t, "%pg_advisory_xact_lock%")
	if n := len(p.cards(t)); n != 0 {
		t.Fatalf("a question was raised while another writer held the deal's lock: %d", n)
	}
	close(release)
	if err := <-held; err != nil {
		t.Fatal(err)
	}
	if err := <-suggested; err != nil {
		t.Fatal(err)
	}
	if n := len(p.pending(t)); n != 1 {
		t.Fatalf("%d questions after the lock was released, want 1", n)
	}
}

// summaryOfTheQuestion is the sentence the owner reads, frozen when the
// question was raised.
func (p *proposalEnv) summaryOfTheQuestion(t *testing.T) string {
	t.Helper()
	return p.scalar(t, `SELECT summary FROM approval WHERE kind = $1 AND target_entity_id = $2`, deals.ProposalStageKind, p.deal)
}

func TestTheQuestionNamesTheStageInTheOwnersLanguage(t *testing.T) {
	for _, tc := range []struct {
		name         string
		translations string
		locale       string
		wantStage    string
		notStage     string
	}{
		{"a German owner and a German stage name", `{"de": "Angebotsphase"}`, "de", "„Angebotsphase“", "Proposal Sent"},
		{"a German owner and a stage with no German name", `{}`, "de", "„Proposal Sent“", "Angebotsphase"},
		{"an English owner and a stage with a German name", `{"de": "Angebotsphase"}`, "en", "“Proposal Sent”", "Angebotsphase"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newProposalEnv(t)
			p.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, p.deal, p.Rep1)
			p.WsExec(t, `UPDATE app_user SET locale = $2 WHERE id = $1`, p.Rep1, tc.locale)
			p.WsExec(t, `UPDATE stage SET name_translations = $2::jsonb WHERE id = $1`, p.stages[4], tc.translations)
			p.handle(t, p.sendOffer(t))
			summary := p.summaryOfTheQuestion(t)
			if !strings.Contains(summary, tc.wantStage) {
				t.Fatalf("the question reads %q, want it to name the stage as %s", summary, tc.wantStage)
			}
			if strings.Contains(summary, tc.notStage) {
				t.Fatalf("the question reads %q, which holds %q", summary, tc.notStage)
			}
		})
	}
}
