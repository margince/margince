// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What a contact wrote, and what a surviving reading of it becomes.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownWords reads the messages each contact WROTE, newest first.
//
// Two conditions make these their own words rather than merely messages about
// them, and both matter to the gate downstream. `direction = 'inbound'` says
// the message came from outside; `activity_participant.role = 'from'` says
// this person is the one it came from. Linked-to alone would admit a message
// somebody else wrote that merely mentions them, and the gate — which binds
// evidence to its author — would then be checking a claim against a source the
// claimed author never typed.
//
// Gated on ActivityContentClause, not the discover clause. The bodies here go
// into a prompt, and the discover gate admits rows whose content the caller may
// not read: a reader allowed to know a message exists is not thereby allowed to
// have it summarised for them.
func ownWords(
	ctx context.Context, tx pgx.Tx, personIDs []ids.PersonID, now time.Time,
) (map[ids.UUID][]proposeroles.Message, error) {
	out := map[ids.UUID][]proposeroles.Message{}
	if len(personIDs) == 0 {
		return out, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// No activity grant is no evidence, and no evidence is no proposal.
		// Empty rather than an error: the caller may still read the deal, and
		// the honest answer to "what did these people say" is that this reader
		// may not know.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	peoplePos := arg(personIDs)
	sincePos := arg(now.AddDate(0, 0, -proposalWindowDays))
	capPos := arg(proposalMessages)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	if scope == "" {
		scope = scopeAll
	}
	// One lateral per person rather than one query per person: the cap is
	// per-contact, so a single ORDER BY over the union would spend the whole
	// budget on the busiest correspondent and read nothing from the others.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT who.id, said.id, coalesce(said.subject, ''), coalesce(said.body, '')
		  FROM unnest($%d::uuid[]) AS who(id)
		  JOIN LATERAL (
		       SELECT a.id, a.subject, a.body
		         FROM activity a
		         JOIN activity_participant ap
		           ON ap.activity_id = a.id AND ap.role = 'from' AND ap.person_id = who.id
		        WHERE a.direction = 'inbound' AND a.archived_at IS NULL
		          AND a.occurred_at >= $%d AND (%s)
		        ORDER BY a.occurred_at DESC, a.id DESC
		        LIMIT $%d) AS said ON TRUE`,
		peoplePos, sincePos, scope, capPos), args...)
	if err != nil {
		return nil, fmt.Errorf("org360: reading what the contacts wrote: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var person, activity ids.UUID
		var subject, body string
		if err := rows.Scan(&person, &activity, &subject, &body); err != nil {
			return nil, err
		}
		out[person] = append(out[person], proposeroles.Message{
			ActivityID: activity.String(), Subject: subject, Body: body,
		})
	}
	return out, rows.Err()
}

// proposalWindowDays bounds how far back a role may be read from.
//
// A buying role is a statement about the deal in front of us. Two years of
// history would let a contact who signed off a budget on a closed deal be read
// as this one's economic buyer, and the reader has no way to tell from the
// card that the evidence is stale.
const proposalWindowDays = 365

// writeProposedSeats commits what survived the gate.
//
// Each seat is written as the reading AGENT rather than the person who pressed
// the button, on that person's behalf: the human's own audit row records the
// decision to ask, and this one carries the machine provenance the committee
// card renders as "read from what they wrote". That captured_by is the whole
// of the marking — the coverage read already looks for exactly this string.
func (s *Service) writeProposedSeats(
	ctx context.Context, dealID ids.DealID,
	kept []proposeroles.Proposal, candidates []proposeroles.Candidate,
) ([]crmcontracts.DealRoleProposalWritten, error) {
	written := []crmcontracts.DealRoleProposalWritten{}
	if len(kept) == 0 {
		return written, nil
	}
	decider, ok := principal.Actor(ctx)
	if !ok {
		return nil, fmt.Errorf("org360: proposing roles without a deciding principal")
	}
	execCtx := principal.WithActor(ctx, principal.Principal{
		Type:       principal.PrincipalSystem,
		ID:         proposeroles.CapturedBy,
		UserID:     decider.UserID,
		OnBehalfOf: decider.UserID,
	})
	named := map[string]string{}
	for _, candidate := range candidates {
		named[candidate.PersonID] = candidate.FullName
	}
	for _, proposal := range kept {
		raw, err := ids.Parse(proposal.PersonID)
		if err != nil {
			// The gate already refused any person this call did not offer, so
			// an unparseable id here means the candidate set itself was built
			// with one — a programming error, not a model one.
			return nil, fmt.Errorf("org360: proposed role for an unreadable person id: %w", err)
		}
		personID := ids.From[ids.PersonKind](raw)
		role := proposal.Role
		seat := people.CreateRelationshipInput{
			Kind:     "deal_stakeholder",
			PersonID: &personID,
			DealID:   &dealID,
			Role:     &role,
			Source:   proposeroles.Source,
		}
		if err := s.commitSeat(execCtx, seat); err != nil {
			return nil, err
		}
		written = append(written, crmcontracts.DealRoleProposalWritten{
			PersonId:         openapi_types.UUID(personID.UUID),
			FullName:         named[proposal.PersonID],
			Role:             proposal.Role,
			EvidenceSnippet:  proposal.EvidenceSnippet,
			SourceActivityId: openapi_types.UUID(mustParseActivity(proposal.SourceID)),
			Confidence:       float32(proposal.Confidence),
		})
	}
	return written, nil
}

// commitSeat writes one seat through the relationship writer.
//
// The module's own writer, not a statement of ours: it is what spells the
// write shape for this table — the domain row, its audit entry and its outbox
// event in one transaction — and a second spelling here would be a second
// answer to how a seat is recorded.
func (s *Service) commitSeat(ctx context.Context, seat people.CreateRelationshipInput) error {
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := s.people.CreateRelationshipTx(ctx, tx, seat)
		return err
	})
}

// mustParseActivity converts a source id the gate has already checked.
//
// The gate keeps only proposals whose source_id is one THIS call supplied, and
// every supplied id came from a uuid column, so a parse failure here is
// unreachable rather than a case to handle. A zero uuid on the wire would be
// the honest shape of "we cannot say", and it is one the client renders as no
// link rather than a broken one.
func mustParseActivity(raw string) ids.UUID {
	parsed, err := ids.Parse(raw)
	if err != nil {
		return ids.UUID{}
	}
	return parsed
}
