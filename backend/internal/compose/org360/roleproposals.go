// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// Reading a deal's buying roles out of what its contacts wrote.
//
// The engine — the prompt, its fence and the gate that stands between a
// model's answer and a customer's record — is compose/proposeroles. This file
// is what feeds it and what writes what survives, and it lives HERE because
// every read it needs is already in this package under the caller's own scope:
// the account's roster, each contact's identity, which deals are visible and
// open, and who already holds a seat. A separate package would have had to
// copy four security-relevant readers to ask the same questions, and a copied
// scope clause is one that stops agreeing with its original.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// Completer is the model seam: the propose-roles lane, or nil.
//
// Nil is not a degraded mode here, unlike every drafting lane in this tree. A
// draft with no model falls back to a template; a buying role has no template,
// because the only thing left to read it from would be the job title — and
// that is the one inference the contract forbids. So an unwired role reads 501
// rather than guessing.
type Completer interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// proposalCandidates is how many contacts one call may read.
//
// The prompt carries every candidate's recent messages, so this bounds the
// call's cost and its latency together. Twelve is the size of a committee a
// rep would name by hand; past it the account's contacts are a mailing list
// rather than a buying group, and reading all of them would spend a premium
// call on people nobody is selling to.
const proposalCandidates = 12

// proposalMessages is how many of a contact's own messages are read.
//
// Newest first, because a role is a statement about the deal as it stands: a
// contact who signed off budgets two years ago and has said nothing since is
// evidence about a different deal.
const proposalMessages = 6

// ProposeRoles reads the deal's buying roles and writes what survives the gate.
//
// The order of operations is the safety. Everything the model sees is
// assembled under the CALLER's own scope first, so a contact they may not see
// never enters the prompt and cannot be proposed for; the gate then refuses
// anything the model returned that the evidence does not support; and only
// what is left is written, attributed to the reading agent rather than to the
// person who pressed the button.
func (s *Service) ProposeRoles(
	ctx context.Context, lane Completer, dealID ids.DealID,
) (crmcontracts.DealRoleProposalResult, error) {
	// Human-only: this spends the workspace's model budget and writes seats a
	// colleague will act on. An agent asking for it would be the product
	// deciding to write its own committee.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	// The write grants, checked BEFORE the model call rather than after it. A
	// caller who cannot create a seat must not spend a premium call to be told
	// so, and the store would refuse the write anyway — under the substituted
	// agent principal, where the refusal would name the wrong actor.
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	now := s.now().UTC()

	var dealName string
	var candidates []proposeroles.Candidate
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		dealName, candidates, err = s.proposalInput(ctx, tx, dealID, now)
		return err
	})
	if err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	if len(candidates) == 0 {
		// Nobody to read. An empty answer, not a failure: an account whose
		// contacts have written nothing has no evidence of a buying role, and
		// saying so is the correct reading.
		return emptyProposalResult(), nil
	}

	proposals, err := readProposals(ctx, lane, dealName, candidates)
	if err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	kept := proposeroles.Gate(proposals, candidates)

	written, err := s.writeProposedSeats(ctx, dealID, kept, candidates)
	if err != nil {
		return crmcontracts.DealRoleProposalResult{}, err
	}
	return crmcontracts.DealRoleProposalResult{
		Written:     written,
		Skipped:     len(proposals) - len(kept),
		GeneratedBy: crmcontracts.Model,
	}, nil
}

// emptyProposalResult is the nothing-to-read answer.
//
// Deterministic rather than Model because no model was asked: reporting the
// lane as the author of an empty answer would credit it with a reading it
// never performed.
func emptyProposalResult() crmcontracts.DealRoleProposalResult {
	return crmcontracts.DealRoleProposalResult{
		Written:     []crmcontracts.DealRoleProposalWritten{},
		Skipped:     0,
		GeneratedBy: crmcontracts.Deterministic,
	}
}

// readProposals asks the model and decodes its answer.
func readProposals(
	ctx context.Context, lane Completer, dealName string, candidates []proposeroles.Candidate,
) ([]proposeroles.Proposal, error) {
	res, err := lane.Complete(ctx, proposeroles.Request(dealName, candidates))
	if err != nil {
		return nil, fmt.Errorf("org360: reading buying roles: %w", err)
	}
	proposals, err := proposeroles.Parse(res.Text)
	if err != nil {
		return nil, fmt.Errorf("org360: reading buying roles: %w", err)
	}
	return proposals, nil
}

// proposalInput assembles what the model may read, under the caller's scope.
//
// Every reader here is the one the coverage card already uses, so a contact
// this refuses to show on the page cannot reach a prompt either.
func (s *Service) proposalInput(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, now time.Time,
) (string, []proposeroles.Candidate, error) {
	orgID, dealName, err := s.visibleOpenDeal(ctx, tx, dealID)
	if err != nil {
		return "", nil, err
	}
	roster, err := people.StrengthForOrgContacts(ctx, tx, orgID, now)
	if err != nil {
		return "", nil, err
	}
	// Ranked the way the People tab ranks, then cut: a contact who has replied
	// is both the likeliest to have said what they do and the one a reader
	// would look at first, so the cut takes the tail rather than an arbitrary
	// twelve by id.
	people.RankContacts(roster)
	if len(roster) > proposalCandidates {
		roster = roster[:proposalCandidates]
	}
	personIDs := make([]ids.PersonID, 0, len(roster))
	for _, contact := range roster {
		personIDs = append(personIDs, contact.PersonID)
	}
	identity, err := contactIdentity(ctx, tx, orgID, personIDs)
	if err != nil {
		return "", nil, err
	}
	held, err := heldSeats(ctx, tx, dealID, now)
	if err != nil {
		return "", nil, err
	}
	messages, err := ownWords(ctx, tx, personIDs, now)
	if err != nil {
		return "", nil, err
	}

	candidates := make([]proposeroles.Candidate, 0, len(personIDs))
	for _, id := range personIDs {
		said := messages[id.UUID]
		if len(said) == 0 {
			// A contact who has written nothing cannot evidence a role. Passing
			// them anyway would put a name and a title in the prompt with no
			// words under it, which is exactly the title-only reading the
			// contract forbids — and the gate would drop whatever came back.
			continue
		}
		who := identity[id]
		candidates = append(candidates, proposeroles.Candidate{
			PersonID:  id.String(),
			FullName:  who.fullName,
			Title:     titleOf(who),
			HoldsRole: held[id.UUID],
			Messages:  said,
		})
	}
	return dealName, candidates, nil
}

// titleOf is the contact's job title, purchased one included.
//
// Same precedence as the roster's: what a human typed wins, a bought claim
// fills a blank. A title is never evidence here, so an empty one costs the
// reading nothing — it is carried only because the prompt says what it is NOT.
func titleOf(who contactCard) string {
	if who.title != nil && *who.title != "" {
		return *who.title
	}
	if who.providerTitle != nil {
		return *who.providerTitle
	}
	return ""
}

// visibleOpenDeal resolves the deal to its account, refusing anything the
// coverage card would refuse to show.
//
// Built on openDealsWhere like every other reader of this question, so a deal
// closed or outside the caller's scope answers NotFound rather than an
// emptier reading: existence stays hidden, and a closed deal has no committee
// left to propose for.
func (s *Service) visibleOpenDeal(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
) (ids.OrganizationID, string, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return ids.OrganizationID{}, "", err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	dealPos := arg(dealID)
	dealScope, err := scopeClause(ctx, "deal", "d", arg)
	if err != nil {
		return ids.OrganizationID{}, "", err
	}
	var orgID ids.OrganizationID
	var name string
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT d.organization_id, d.name FROM deal d
		 WHERE d.id = $%d AND d.status = 'open' AND d.archived_at IS NULL
		   AND (%s)`, dealPos, dealScope), args...).Scan(&orgID, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.OrganizationID{}, "", apperrors.ErrNotFound
	}
	if err != nil {
		return ids.OrganizationID{}, "", fmt.Errorf("org360: reading the deal to propose roles for: %w", err)
	}
	return orgID, name, nil
}

// heldSeats is who already has a role on this deal.
//
// From deals.Stakeholders, the same reader the committee board renders, so
// "already answered" means the same thing on both. Its person row scope means
// a seat the caller cannot see is absent here — which makes HoldsRole false
// for a seat that exists, so the gate is not the only thing standing between a
// proposal and a duplicate. The write re-checks under the row itself.
func heldSeats(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, now time.Time,
) (map[ids.UUID]bool, error) {
	seats, err := deals.Stakeholders(ctx, tx, dealID, now)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		// Without the edge grant the caller cannot learn who sits on the deal.
		// Reading on regardless would be the disclosure by another route, so
		// the seats stay unknown and every write re-checks the row itself.
		return map[ids.UUID]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(seats))
	for _, seat := range seats {
		if seat.Role != "" {
			out[seat.PersonID] = true
		}
	}
	return out, nil
}
