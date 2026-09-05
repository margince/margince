// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

// The buying-role reading against a real database.
//
// Every rule this endpoint rests on is a SQL predicate — who wrote a message,
// whose messages a caller may read, which deals are visible. None of them can
// fail in a unit test, because none of them exists outside a query. The
// impersonation case below is the reason this file exists: removing the
// authorship predicate leaves every unit test green.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	org360svc "github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// scriptedLane answers with a fixed reply and records what it was asked.
type scriptedLane struct {
	reply string
	asked []model.Request
}

func (l *scriptedLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.asked = append(l.asked, req)
	return model.Response{Text: l.reply}, nil
}

// seedRoleDeal sets up an account with one open deal.
func seedRoleDeal(t *testing.T, e *integration.Env) (org, deal ids.UUID) {
	t.Helper()
	org = e.SeedOrg(t, "Brandt GmbH", nil)
	pipeline, openStage, _ := integration.DealFixture(t, e)
	deal = e.SeedDeal(t, "Retrofit 2026", pipeline, openStage, nil)
	e.WsExec(t, `UPDATE deal SET organization_id = $1 WHERE id = $2`, org, deal)
	return org, deal
}

// wrote records that a person AUTHORED a message to this account.
//
// Three facts, and the endpoint needs all three. The activity_link to the
// PERSON is what the roster reads; the link to the ORGANIZATION is what makes
// it this account's correspondence rather than any message the contact
// happened to send; and activity_participant role='from' is what says they
// wrote it rather than merely being named in it.
func wrote(t *testing.T, e *integration.Env, person, org ids.UUID, subject, body string, daysAgo int) ids.UUID {
	t.Helper()
	owner := integration.OwnerConn(t)
	mail := integration.AccountMailDirectedAt(t, owner, e.WS, subject, "inbound",
		org360Clock.AddDate(0, 0, -daysAgo))
	e.WsExec(t, `UPDATE activity SET body = $1 WHERE id = $2`, body, mail)
	integration.LinkActivity(t, owner, mail, "person", person)
	integration.LinkToOrg(t, e, mail, org)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, person_id, role)
		VALUES ($1, $2, 'from')`, mail, person)
	return mail
}

// A role read out of a contact's own sentence becomes a real seat, marked as
// the product's reading and carrying the words it was read from.
func TestAReadRoleBecomesASeatTheCommitteeMarks(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Chief Financial Officer")
	source := wrote(t, e, buyer, org, "Re: Angebot",
		"I sign off the budget for this, so send the figures to me directly.", 3)

	lane := &scriptedLane{reply: `{"proposals":[{
		"person_id":"` + buyer.String() + `","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send",
		"source_id":"` + source.String() + `","confidence":0.9}]}`}

	got, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal})
	if err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	if len(got.Written) != 1 {
		t.Fatalf("wrote %d seats, want one: %+v", len(got.Written), got.Written)
	}
	if got.Written[0].Role != "economic_buyer" {
		t.Fatalf("wrote role %q", got.Written[0].Role)
	}
	if got.Written[0].EvidenceSnippet == "" {
		t.Fatal("the seat carries no evidence, so a reader cannot check it")
	}

	// The committee read must now mark it, from the row's own captured_by —
	// which is the whole of the marking, with no second column to set.
	coverage, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	if coverage.Committee == nil || len(coverage.Committee.Seats) != 1 {
		t.Fatalf("the committee does not carry the written seat: %+v", coverage.Committee)
	}
	seat := coverage.Committee.Seats[0]
	if seat.AiSuggested == nil || !*seat.AiSuggested {
		t.Fatal("the written seat is not marked as the product's reading")
	}
}

// ONE CONTACT CANNOT SPEAK FOR ANOTHER.
//
// Both contacts sit in one prompt, so a sender who writes an instruction into
// their own email could otherwise hand a role to a colleague they have never
// spoken for.
//
// What holds this is the GATE's author check, not the read below it: reverting
// role='from' alone leaves this green, and reverting the gate's check alone
// turns it red. That is worth stating rather than leaving for the next reader
// to discover, because the two guards answer different questions and only one
// of them answers this one. TestOnlyWhatAContactWroteIsOfferedAsTheirEvidence
// is what pins the read.
func TestOneContactCannotAssignARoleToAnother(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	attacker := e.SeedPerson(t, "Mallory Vance", nil)
	victim := e.SeedPerson(t, "Jan Roth", nil)
	employ(t, e, attacker, org, "Consultant")
	employ(t, e, victim, org, "Head of Fleet")

	// The attacker writes the sentence. The victim is on the same thread as a
	// RECIPIENT — the ordinary shape of a message with two people on it, and
	// the shape an author check has to see through: they are a participant and
	// they are linked, they simply did not write it.
	source := wrote(t, e, attacker, org, "Re: Retrofit",
		"I sign off the budget for this, so send it to me directly.", 2)
	integration.LinkActivity(t, integration.OwnerConn(t), source, "person", victim)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, person_id, role)
		VALUES ($1, $2, 'to')`, source, victim)
	wrote(t, e, victim, org, "Re: Termin", "Thursday afternoon works for me.", 1)

	lane := &scriptedLane{reply: `{"proposals":[{
		"person_id":"` + victim.String() + `","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send",
		"source_id":"` + source.String() + `","confidence":1}]}`}

	got, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal})
	if err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	if len(got.Written) != 0 {
		t.Fatalf("one contact's message assigned a role to another: %+v", got.Written)
	}
	if got.Skipped != 1 {
		t.Fatalf("the refusal was not counted: skipped=%d", got.Skipped)
	}
}

// A seat somebody typed is a human's answer to this question, and the reading
// may only fill a hole.
func TestAReadRoleNeverOverwritesASeatAPersonTyped(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	held := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, held, org, "Chief Financial Officer")
	source := wrote(t, e, held, org, "Re: Angebot",
		"I sign off the budget for this, so send the figures to me directly.", 3)
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'champion', 'manual', 'human:x')`, held, deal)

	lane := &scriptedLane{reply: `{"proposals":[{
		"person_id":"` + held.String() + `","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send",
		"source_id":"` + source.String() + `","confidence":0.95}]}`}

	got, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal})
	if err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	if len(got.Written) != 0 {
		t.Fatalf("overwrote a role a person recorded: %+v", got.Written)
	}
	// The typed seat is untouched and still reads as a human's.
	coverage, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	seat := coverage.Committee.Seats[0]
	if seat.Role != "champion" {
		t.Fatalf("the typed seat now reads %q", seat.Role)
	}
	if seat.AiSuggested != nil && *seat.AiSuggested {
		t.Fatal("a seat a person typed is marked as the product's reading")
	}
}

// A contact who has written nothing never reaches the prompt. Sending them
// with a bare name and title is the title-only reading the contract forbids,
// and it is the one shape a model is most likely to answer wrongly.
func TestAContactWithNoWordsNeverReachesThePrompt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	silent := e.SeedPerson(t, "Dietmar Rietsch", nil)
	spoke := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, silent, org, "Managing Director")
	employ(t, e, spoke, org, "Procurement")
	wrote(t, e, spoke, org, "Re: Angebot", "We will review the scope this week.", 2)

	lane := &scriptedLane{reply: `{"proposals":[]}`}
	if _, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal}); err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	if len(lane.asked) != 1 {
		t.Fatalf("issued %d model calls, want one", len(lane.asked))
	}
	prompt := lane.asked[0].Messages[0].Content
	if strings.Contains(prompt, "Dietmar Rietsch") {
		t.Fatal("a contact who has written nothing was sent to the model with only a title")
	}
	if !strings.Contains(prompt, "Ute Sommer") {
		t.Fatal("the contact who did write was not sent")
	}
}

// A closed deal has no committee left to propose for, and a deal outside the
// caller's scope must not be distinguishable from one that does not exist.
func TestProposingRolesOnAnUnreachableDealIsNotFound(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	_, deal := seedRoleDeal(t, e)
	e.WsExec(t, `UPDATE deal SET status = 'lost', closed_at = now(),
		lost_reason = 'went_elsewhere' WHERE id = $1`, deal)

	lane := &scriptedLane{reply: `{"proposals":[]}`}
	_, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal})
	if err == nil {
		t.Fatal("a closed deal accepted a reading")
	}
	if len(lane.asked) != 0 {
		t.Fatal("a model call was spent on a deal that could not be read")
	}
}

var _ org360svc.Completer = (*scriptedLane)(nil)

// The write runs as the reading agent, and a system principal is UNBOUNDED —
// the store's write-authority check returns for it without consulting the row.
// So the caller's authority over this deal has to be established while they are
// still the principal. Without that, a rep who can merely READ a colleague's
// deal seats a stakeholder they could not add through the ordinary endpoint.
func TestAReaderOfSomebodyElsesDealCannotSeatThroughTheReading(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Chief Financial Officer")
	source := wrote(t, e, buyer, org, "Re: Angebot",
		"I sign off the budget for this, so send the figures to me directly.", 3)

	// The deal belongs to a rep in ANOTHER team, and our caller is bounded to
	// their own — they may read it through the share below, and no more.
	e.WsExec(t, `UPDATE deal SET owner_id = $1 WHERE id = $2`, e.Rep3, deal)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roleProposalReadOnlyPerms)

	lane := &scriptedLane{reply: `{"proposals":[{
		"person_id":"` + buyer.String() + `","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send",
		"source_id":"` + source.String() + `","confidence":0.9}]}`}

	_, err := svc.ProposeRoles(rep, lane, ids.DealID{UUID: deal})
	if err == nil {
		t.Fatal("a rep seated a stakeholder on a deal they may only read")
	}
	if seats := seatsOn(t, deal); seats != 0 {
		t.Fatalf("%d seats were written despite the refusal", seats)
	}
}

// A contact can be a stakeholder on two deals at one company and correspond
// about both. A sentence approving the budget for ONE transaction must not be
// quotable as evidence about the other: the quote is genuine, so nothing
// downstream can catch it.
func TestEvidenceFromAnotherDealDoesNotReachThePrompt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Chief Financial Officer")

	// The sentence, written about an account this deal has nothing to do with.
	elsewhere := e.SeedOrg(t, "Globex AG", nil)
	wrote(t, e, buyer, elsewhere, "Re: Globex",
		"I sign off the budget for this, so send it to me directly.", 4)
	// And one that IS this account's correspondence.
	wrote(t, e, buyer, org, "Re: Retrofit", "We will review the scope this week.", 2)

	lane := &scriptedLane{reply: `{"proposals":[]}`}
	if _, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal}); err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	prompt := lane.asked[0].Messages[0].Content
	if strings.Contains(prompt, "I sign off the budget") {
		t.Fatal("a sentence about another account was offered as evidence about this deal")
	}
	if !strings.Contains(prompt, "We will review the scope") {
		t.Fatal("this account's own correspondence never reached the prompt")
	}
}

// Reading what the contacts wrote IS this endpoint. A caller without the
// activity grant is not owed a thinner answer — an empty reading reads as
// "nobody here has said anything about who buys", which is a claim about the
// account rather than about their permissions.
func TestWithoutTheActivityGrantTheReadingIsRefusedNotEmptied(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Chief Financial Officer")
	wrote(t, e, buyer, org, "Re: Angebot", "I sign off the budget for this one.", 3)

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoActivityPerms)
	lane := &scriptedLane{reply: `{"proposals":[]}`}
	if _, err := svc.ProposeRoles(blind, lane, ids.DealID{UUID: deal}); err == nil {
		t.Fatal("a reader who may not see the messages got a reading of them anyway")
	}
	if len(lane.asked) != 0 {
		t.Fatal("a model call was spent for a caller who may not read the evidence")
	}
}

// roleProposalReadOnlyPerms is a rep who may READ everything this endpoint
// reads and CREATE a relationship, but whose row scope is their own team.
//
// The relationship grant is present on purpose: without it the refusal would
// come from the object check and prove nothing about row-level write
// authority, which is the hole this fixture exists to expose.
var roleProposalReadOnlyPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"organization":          {Read: true},
		"person":                {Read: true},
		"deal":                  {Read: true, Update: true},
		"relationship":          {Read: true, Create: true},
		"activity":              {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// seatsOn counts the deal's live stakeholder rows, unscoped.
func seatsOn(t *testing.T, deal ids.UUID) int {
	t.Helper()
	var seats int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM relationship
		  WHERE kind = 'deal_stakeholder' AND deal_id = $1`, deal).Scan(&seats); err != nil {
		t.Fatalf("counting seats: %v", err)
	}
	return seats
}

// The READ's own obligation: a contact's evidence is what they WROTE.
//
// Distinct from the gate's author check, which refuses a mismatch between a
// proposal and its source. This asks the question one step earlier — what the
// prompt is told each contact said — and the difference shows when a contact
// is merely a recipient. Offering them a colleague's sentence as their own
// words invites exactly the confusion the gate then has to catch, and a model
// asked a leading question answers it.
func TestOnlyWhatAContactWroteIsOfferedAsTheirEvidence(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	author := e.SeedPerson(t, "Mallory Vance", nil)
	recipient := e.SeedPerson(t, "Jan Roth", nil)
	employ(t, e, author, org, "Consultant")
	employ(t, e, recipient, org, "Head of Fleet")

	// One message, two people on it: one wrote it, one received it.
	source := wrote(t, e, author, org, "Re: Retrofit",
		"I sign off the budget for this, so send it to me directly.", 2)
	integration.LinkActivity(t, integration.OwnerConn(t), source, "person", recipient)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, person_id, role)
		VALUES ($1, $2, 'to')`, source, recipient)
	wrote(t, e, recipient, org, "Re: Termin", "Thursday afternoon works for me.", 1)

	lane := &scriptedLane{reply: `{"proposals":[]}`}
	if _, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal}); err != nil {
		t.Fatalf("proposing roles: %v", err)
	}
	prompt := lane.asked[0].Messages[0].Content
	// The sentence is offered ONCE. The prompt lists each contact with the
	// messages attributed to them, so a second copy is the same words offered
	// again under somebody who did not write them — which is the leading
	// question this read must not ask.
	if got := strings.Count(prompt, "I sign off the budget"); got != 1 {
		t.Fatalf("the sentence is offered %d times, want once (under its author only)", got)
	}
	// And it sits in the AUTHOR's block: the block boundary is their person id,
	// and the recipient's own message is what follows theirs.
	authored := blockFor(t, prompt, author.String(), recipient.String())
	if !strings.Contains(authored, "I sign off the budget") {
		t.Fatal("the author's own sentence is not offered under them")
	}
	received := blockFor(t, prompt, recipient.String(), author.String())
	if strings.Contains(received, "I sign off the budget") {
		t.Fatal("a message the contact only received was offered as words they wrote")
	}
	if !strings.Contains(received, "Thursday afternoon works for me") {
		t.Fatal("the recipient's own words are missing, so the assertion above is vacuous")
	}
}

// blockFor returns one candidate's slice of the prompt: from their person id
// up to the next candidate's, whichever order the ranking put them in.
func blockFor(t *testing.T, prompt, mine, theirs string) string {
	t.Helper()
	start := strings.Index(prompt, mine)
	if start < 0 {
		t.Fatalf("contact %s never reached the prompt", mine)
	}
	rest := prompt[start:]
	if end := strings.Index(rest, theirs); end >= 0 {
		return rest[:end]
	}
	return rest
}

// CONFIRMING IS AN EDIT THAT CHANGES NOTHING, AND IT STILL CLEARS THE MARK.
//
// The card's Confirm sends the seat's role back unchanged, and the whole
// design rests on the relationship writer reassigning captured_by to whoever
// edits. If that writer ever short-circuits a patch whose fields all match,
// Confirm becomes a button that reports success and changes nothing — the mark
// stays, the reader presses it again, and nothing they do ever takes.
func TestConfirmingAReadRoleClearsTheMarkWithoutChangingIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	buyer := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, buyer, org, "Chief Financial Officer")
	source := wrote(t, e, buyer, org, "Re: Angebot",
		"I sign off the budget for this, so send the figures to me directly.", 3)

	lane := &scriptedLane{reply: `{"proposals":[{
		"person_id":"` + buyer.String() + `","role":"economic_buyer",
		"evidence_snippet":"I sign off the budget for this, so send",
		"source_id":"` + source.String() + `","confidence":0.9}]}`}
	if _, err := svc.ProposeRoles(ctx, lane, ids.DealID{UUID: deal}); err != nil {
		t.Fatalf("proposing roles: %v", err)
	}

	before, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	seat := before.Committee.Seats[0]
	if seat.AiSuggested == nil || !*seat.AiSuggested {
		t.Fatal("the written seat is not marked, so the clearing below proves nothing")
	}
	if seat.RelationshipId == nil || seat.RelationshipVersion == nil {
		t.Fatal("the seat carries no row to patch, so the card cannot confirm it")
	}

	// Exactly what the card sends: the same role, nothing else.
	role := seat.Role
	if _, err := e.People.UpdateRelationship(ctx, ids.UUID(*seat.RelationshipId),
		people.UpdateRelationshipInput{Role: &role}); err != nil {
		t.Fatalf("confirming the seat: %v", err)
	}

	after, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("re-reading coverage: %v", err)
	}
	confirmed := after.Committee.Seats[0]
	if confirmed.AiSuggested != nil && *confirmed.AiSuggested {
		t.Fatal("the mark survived a human's edit, so Confirm reports success and does nothing")
	}
	if confirmed.Role != role {
		t.Fatalf("confirming changed the role to %q", confirmed.Role)
	}
}

// A PERSON CAN HOLD TWO ROLES ON ONE DEAL, and each card must address its own
// row.
//
// The table's uniqueness key is (deal_id, person_id, role), so this is a shape
// the database permits. Keyed by person alone, both cards carried whichever
// row the scan returned last — so Confirm on one patched the other, changing a
// role the reader was not looking at or colliding with the uniqueness index.
func TestASeatCardAddressesItsOwnRowWhenAPersonHoldsTwoRoles(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	both := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, both, org, "Chief Financial Officer")
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x')`, both, deal)
	e.WsExec(t, `INSERT INTO relationship (kind, person_id, deal_id, role, source, captured_by)
		VALUES ('deal_stakeholder', $1, $2, 'champion', 'ai_proposal', 'agent:propose_roles')`,
		both, deal)

	got, err := svc.Coverage(ctx, ids.OrganizationID{UUID: org})
	if err != nil {
		t.Fatalf("reading coverage: %v", err)
	}
	rows := map[string]string{}
	marked := map[string]bool{}
	for _, seat := range got.Committee.Seats {
		if seat.RelationshipId == nil {
			t.Fatalf("seat %q carries no row to patch", seat.Role)
		}
		rows[seat.Role] = seat.RelationshipId.String()
		marked[seat.Role] = seat.AiSuggested != nil && *seat.AiSuggested
	}
	if len(rows) != 2 {
		t.Fatalf("read %d seats for a person holding two roles: %+v", len(rows), rows)
	}
	if rows["champion"] == rows["economic_buyer"] {
		t.Fatalf("both cards address the same row (%s) — confirming one patches the other",
			rows["champion"])
	}
	// And the provenance follows the ROW, not the person: one seat was typed
	// and one was read, and a mark shared between them would offer to confirm
	// a colleague's own answer.
	if !marked["champion"] {
		t.Fatal("the read seat is not marked")
	}
	if marked["economic_buyer"] {
		t.Fatal("a seat a person typed is marked as the product's reading")
	}
}

// A DEAL THE CALLER NAMED AND CANNOT READ IS A REFUSAL, not an account-wide
// draft.
//
// Dropping it silently returns 200 with a message about the wrong subject,
// which the rep then sends to a colleague believing it says what they asked
// for. An ABSENT deal_id is the account-wide case, and only that.
func TestAnIntroRequestRefusesADealTheCallerCannotRead(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org, deal := seedRoleDeal(t, e)
	contact := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, contact, org, "Chief Financial Officer")
	wrote(t, e, contact, org, "Re: Angebot", "We will review the scope this week.", 2)
	// A REAL route, so the refusal below is about the deal rather than about a
	// colleague who cannot reach anybody. Without it this test passed with the
	// bug restored, because introRoute refused first. The route is a SECOND
	// rep's: an introduction is asked of somebody else, so a route belonging to
	// the caller would be refused before the deal was ever read.
	e.WsExec(t, `INSERT INTO graph_interaction_edge
			(user_id, person_id, last_at, count_90d, in_count_90d, out_count_90d)
		VALUES ($1, $2, $3, 20, 10, 10)`,
		e.Rep2, contact, org360Clock.AddDate(0, 0, -2))

	// A reader who may see the account, its people and their routes — and no
	// deals at all.
	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoDealPerms)
	_, err := svc.IntroRequestDraft(blind, nil, ids.OrganizationID{UUID: org},
		org360svc.IntroRequest{
			PersonID:  ids.From[ids.PersonKind](contact),
			ViaUserID: ids.From[ids.UserKind](e.Rep2),
			DealID:    ptrDeal(deal),
		})
	if err == nil {
		t.Fatal("a deal the caller cannot read was silently dropped and a draft returned")
	}
}

func ptrDeal(id ids.UUID) *ids.DealID {
	deal := ids.From[ids.DealKind](id)
	return &deal
}

// And the same caller, asking for no deal in particular, gets their draft. Without
// this the refusal above would pass against an endpoint that refuses everybody.
func TestAnIntroRequestWithoutADealStillDraftsForTheSameCaller(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org, _ := seedRoleDeal(t, e)
	contact := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, contact, org, "Chief Financial Officer")
	wrote(t, e, contact, org, "Re: Angebot", "We will review the scope this week.", 2)
	e.WsExec(t, `INSERT INTO graph_interaction_edge
			(user_id, person_id, last_at, count_90d, in_count_90d, out_count_90d)
		VALUES ($1, $2, $3, 20, 10, 10)`,
		e.Rep2, contact, org360Clock.AddDate(0, 0, -2))

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoDealPerms)
	draft, err := svc.IntroRequestDraft(blind, nil, ids.OrganizationID{UUID: org},
		org360svc.IntroRequest{
			PersonID:  ids.From[ids.PersonKind](contact),
			ViaUserID: ids.From[ids.UserKind](e.Rep2),
		})
	if err != nil {
		t.Fatalf("an account-wide introduction was refused: %v", err)
	}
	if !strings.Contains(draft.Body, "Ute Sommer") {
		t.Fatalf("the draft does not name who is to be met:\n%s", draft.Body)
	}
	// No lane was given, so the template wrote it and says so.
	if draft.GeneratedBy != "deterministic" {
		t.Fatalf("a draft written with no lane is credited to %q", draft.GeneratedBy)
	}
}

// AN INTRODUCTION IS ASKED OF SOMEBODY ELSE.
//
// The routes this draft is written from rank everyone on our side who
// corresponds with the contact, and the reader is one of them — so the way in
// the page recommends can be the reader's own relationship. Drafting from that
// wrote a letter asking its own sender for a favour, at the workspace's model
// budget, and the reader had a route the whole time.
func TestAnIntroRequestRefusesTheReaderAsTheirOwnIntroducer(t *testing.T) {
	e := integration.Setup(t)
	svc := org360Service(e)

	org, _ := seedRoleDeal(t, e)
	contact := e.SeedPerson(t, "Ute Sommer", nil)
	employ(t, e, contact, org, "Chief Financial Officer")
	wrote(t, e, contact, org, "Re: Angebot", "We will review the scope this week.", 2)
	// The caller's OWN route, which is the shape the page offers when the
	// warmest relationship with this contact is the reader's.
	e.WsExec(t, `INSERT INTO graph_interaction_edge
			(user_id, person_id, last_at, count_90d, in_count_90d, out_count_90d)
		VALUES ($1, $2, $3, 20, 10, 10)`,
		e.Rep1, contact, org360Clock.AddDate(0, 0, -2))

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, org360NoDealPerms)
	_, err := svc.IntroRequestDraft(reader, nil, ids.OrganizationID{UUID: org},
		org360svc.IntroRequest{
			PersonID:  ids.From[ids.PersonKind](contact),
			ViaUserID: ids.From[ids.UserKind](e.Rep1),
		})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("a reader drafted an introduction from themselves: %v", err)
	}
}
