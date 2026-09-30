// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for cold_start/sitereadmessage — the conversation an
// administrator has with Margince about the dossier its crawl just read.
//
// It certifies the shipped path rather than a description of it: the request
// comes from companyReadAnswerRequest and the reply is judged by the gate
// newCompanyReadGate builds, both of which the transport uses for the same turn.
// A case that rebuilt either would measure a copy, and a copy stays green
// through the change that breaks the original.
//
// This site is MULTI-TURN, and the kind names the conversation rather than the
// number of calls: the model is stateless, so the whole prior conversation is
// replayed as messages of the ONE request this site sends. The case replays
// exactly the turns the transport would have replayed, through
// companyReadConversation — the same mapping, which is also the bound on how
// many turns a call may carry.
//
// What makes this site worth certifying separately from the other onboarding
// conversations is its gate. The other acts may not propose company changes at
// all; this one may, and only the ones the administrator actually asked for.
// That authorization is derived from what the human said — this message and the
// conversation behind it — so the SAME reply is a correct correction in one
// conversation and a confused-deputy action in the next: a change nobody asked
// for arrives at the confirm-first queue indistinguishable from one they did,
// which is where an approval stops being a decision. The dossier is crawled web
// text and can argue for a change in as many words; the gate, not the prompt, is
// what refuses it. So the case's gate is built from the fixture it sent, and
// never from anything else.
//
// What the expectation MEANS here is the company conversations' shared claim —
// the register the reply answers in and the changes it proposes — read by
// readCompanyConversationExpectation.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// siteReadMessageSite names this site in every refusal it writes, so a corpus
// author reading one knows which scenario to open.
const siteReadMessageSite = "cold_start/sitereadmessage"

// companyReadMessageFixture is ONE turn of the dossier conversation, in exactly
// what the transport hands the answer path: what the administrator just said,
// the conversation it follows, and the dossier the crawl grounded.
//
// The dossier arrives assembled rather than as the site read it came from,
// because the certified thing is the prompt built from it, not the database read
// that produced it. What the server's assembly guarantees about it is enforced
// at Prepare instead: numbering, grounding and bounds are what the model is
// shown and what the gate looks up, so a fixture outside them describes a call
// the product cannot make.
//
// The turns are carried in the transport's own wire shape so the case bounds
// them the way the transport bounds them.
type companyReadMessageFixture struct {
	Message  string                   `json:"message"`
	History  []companyReadFixtureTurn `json:"history"`
	Evidence []companyReadEvidence    `json:"evidence"`
}

// companyReadFixtureTurn is one replayed turn as a scenario writes it: the wire
// turn, and on Margince's turn the offer the server recorded when it made it.
// The wire carries no offer, so this is the only place a scenario can say one
// was made.
type companyReadFixtureTurn struct {
	crmcontracts.CompanySiteReadConversationTurn
	Offer *companyReadOffer `json:"offer,omitempty"`
}

// companyReadFixtureDraftVersion is the dossier draft every fixture's offers
// are recorded against and answered over: a scenario certifies the model, and
// a re-read dossier between two of its turns is not a question a model answers.
const companyReadFixtureDraftVersion = 1

// companyReadFixtureConversation maps the scenario's turns through the
// transport's own mapping and derives the standing offer the way the transport
// does: the last recorded offer, standing only if the conversation ends on the
// turn that made it. An offer the gate would have refused when it was made
// describes a slot the server never writes, so it is refused here.
func companyReadFixtureConversation(site string, turns []companyReadFixtureTurn, evidence []companyReadEvidence) ([]model.Message, *companyReadOffer, error) {
	wire := make([]crmcontracts.CompanySiteReadConversationTurn, len(turns))
	var recorded *contacts.SiteReadOffer
	for i, turn := range turns {
		wire[i] = turn.CompanySiteReadConversationTurn
		if turn.Offer == nil {
			continue
		}
		if turn.Role != crmcontracts.CompanySiteReadConversationTurnRoleAssistant {
			return nil, nil, fmt.Errorf("%s: turn %d is the administrator's and carries an offer; only Margince makes one", site, i+1)
		}
		cited := make(map[string]struct{}, len(turn.Offer.SourceIDs))
		for _, sourceID := range turn.Offer.SourceIDs {
			cited[sourceID] = struct{}{}
		}
		if err := validateCompanyReadOffers([]companyReadOffer{*turn.Offer}, cited, companyReadEvidenceIndex(evidence)); err != nil {
			return nil, nil, fmt.Errorf("%s: turn %d records an offer the server would have refused: %w", site, i+1, err)
		}
		offer := recordedOffer(*turn.Offer, turn.Message, companyReadFixtureDraftVersion)
		recorded = &offer
	}
	history, err := companyReadConversation(&wire)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: the fixture's history is not one the transport accepts: %w", site, err)
	}
	return history, standingOffer(recorded, history, companyReadFixtureDraftVersion), nil
}

// companyReadMessageCases serves the site that answers an administrator about
// the dossier a site read produced.
type companyReadMessageCases struct{}

func (companyReadMessageCases) Site() aitasks.Site {
	return aitasks.Site{
		Task:    ai.TaskColdStart,
		Variant: "sitereadmessage",
		Kind:    ai.SiteKindMultiTurn,
	}
}

// Prepare turns one dossier turn and what the scenario expects of it into a
// runnable case, deriving the gate from the same message, history and dossier
// the request is built from — which is the whole reason Prepare exists.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (companyReadMessageCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f companyReadMessageFixture
	if err := decodeCompanyConversationScenario(fixture, &f); err != nil {
		return nil, fmt.Errorf("%s: the fixture is not the shape this site takes: %w", siteReadMessageSite, err)
	}
	if err := refuseUnsendableCompanyMessage(siteReadMessageSite, f.Message); err != nil {
		return nil, err
	}
	if err := refuseUnassemblableDossier(siteReadMessageSite, f.Evidence); err != nil {
		return nil, err
	}
	history, offer, err := companyReadFixtureConversation(siteReadMessageSite, f.History, f.Evidence)
	if err != nil {
		return nil, err
	}
	message := strings.TrimSpace(f.Message)
	gate := newCompanyReadGate(message, history, f.Evidence, offer)
	want, err := readCompanyConversationExpectation(siteReadMessageSite, expected, gate)
	if err != nil {
		return nil, err
	}
	return &companyReadMessageCase{
		message: message, history: history, evidence: f.Evidence, offer: offer, gate: gate, expected: want,
	}, nil
}

// companyReadMessageCase is one dossier turn ready to be answered, closed over
// the gate built from that same turn.
type companyReadMessageCase struct {
	message  string
	history  []model.Message
	evidence []companyReadEvidence
	offer    *companyReadOffer
	gate     companyReadGate
	expected companyConversationExpectation
}

// Run issues the one request this site sends, the replayed conversation inside
// it. It sends it bare: production wraps the same request in the shape-retry
// when the brain supports one, and a case that retried would certify the answer
// a model gives after being told to try again rather than the answer it gives.
func (c *companyReadMessageCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req, err := companyReadAnswerRequest(c.message, c.history, c.evidence, c.offer)
	if err != nil {
		return aitasks.Trace{}, fmt.Errorf("%s: %w", siteReadMessageSite, err)
	}
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("%s: %w", siteReadMessageSite, err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate judges the reply with the gate this turn was sent under, in the
// answer path's own order.
func (c *companyReadMessageCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	return evaluateCompanyConversationReply(c.gate, c.expected, trace)
}
