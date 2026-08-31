// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What a contact wrote, and what a surviving reading of it becomes.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/proposeroles"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
// And the message must belong to THIS account's correspondence — linked to the
// organization or to one of its deals. A contact can be a stakeholder on two
// deals at one company and correspond about both; without this bound, a
// sentence approving the budget for one transaction is quotable as evidence
// about the other, and the quote is genuine, so nothing downstream can catch
// it.
//
// Gated on ActivityContentClause, not the discover clause. The bodies here go
// into a prompt, and the discover gate admits rows whose content the caller may
// not read: a reader allowed to know a message exists is not thereby allowed to
// have it summarised for them.
func ownWords(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
	personIDs []ids.PersonID, now time.Time,
) (map[ids.UUID][]proposeroles.Message, error) {
	out := map[ids.UUID][]proposeroles.Message{}
	if len(personIDs) == 0 {
		return out, nil
	}
	// The activity grant is REFUSED, not softened. Reading what the contacts
	// wrote is the whole of this endpoint, so a caller without it is not
	// getting a thinner answer — they are being told an empty account. That
	// reads exactly like "nobody here has said anything about who buys", which
	// is a fact about the account rather than about their permissions, and it
	// is the wrong one.
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	peoplePos := arg(personIDs)
	orgPos := arg(orgID)
	sincePos := arg(now.AddDate(0, 0, -proposalWindowDays))
	capPos := arg(proposalMessages)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
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
		          AND EXISTS (
		              SELECT 1 FROM activity_link tie
		               WHERE tie.activity_id = a.id
		                 AND (tie.organization_id = $%d
		                      OR tie.deal_id IN (SELECT id FROM deal
		                                          WHERE organization_id = $%d)))
		        ORDER BY a.occurred_at DESC, a.id DESC
		        LIMIT $%d) AS said ON TRUE`,
		peoplePos, sincePos, scope, orgPos, orgPos, capPos), args...)
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

// writeProposedSeats commits what survived the gate, in ONE transaction.
//
// One transaction because the endpoint returns one aggregate answer. Committing
// each seat separately would let the second fail with the first already
// written, and the caller — told only "error" — would retry into a committee
// that has silently half-changed under them.
//
// Each seat is written as the reading AGENT rather than the person who pressed
// the button, on that person's behalf: the human's own audit row records the
// decision to ask, and this one carries the machine provenance the committee
// card renders as "read from what they wrote". That captured_by is the whole of
// the marking — the coverage read already looks for exactly this string.
//
// The substitution is for PROVENANCE and never for authority: a system
// principal is unbounded, so the caller's write authority over this deal was
// established while they were still the principal, before any of this ran.
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
	err := database.WithWorkspaceTx(execCtx, s.pool, func(tx pgx.Tx) error {
		// Re-read who holds a seat, INSIDE the writing transaction. The check
		// before the prompt was a snapshot, and the model call takes seconds:
		// a colleague who names a champion while it runs would otherwise be
		// seconded by a reading that started before they answered. The table's
		// uniqueness key includes the role, so the database does not catch it.
		taken, err := seatedNow(execCtx, tx, dealID)
		if err != nil {
			return err
		}
		written = written[:0]
		for _, proposal := range kept {
			if taken[proposal.PersonID] {
				continue
			}
			row, err := s.seatFrom(execCtx, tx, dealID, proposal, named)
			if err != nil {
				return err
			}
			written = append(written, row)
		}
		if len(written) == 0 {
			return nil
		}
		return recordEvidence(execCtx, tx, dealID, written)
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

// seatFrom writes one seat and describes what was written.
func (s *Service) seatFrom(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
	proposal proposeroles.Proposal, named map[string]string,
) (crmcontracts.DealRoleProposalWritten, error) {
	raw, err := ids.Parse(proposal.PersonID)
	if err != nil {
		// The gate already refused any person this call did not offer, so an
		// unparseable id here means the candidate set itself was built with
		// one — a programming error, not a model one.
		return crmcontracts.DealRoleProposalWritten{},
			fmt.Errorf("org360: proposed role for an unreadable person id: %w", err)
	}
	personID := ids.From[ids.PersonKind](raw)
	role := proposal.Role
	// The module's own writer, not a statement of ours: it is what spells the
	// write shape for this table — the domain row, its audit entry and its
	// outbox event in one transaction — and a second spelling here would be a
	// second answer to how a seat is recorded.
	if _, err := s.people.CreateRelationshipTx(ctx, tx, people.CreateRelationshipInput{
		Kind:     "deal_stakeholder",
		PersonID: &personID,
		DealID:   &dealID,
		Role:     &role,
		Source:   proposeroles.Source,
	}); err != nil {
		return crmcontracts.DealRoleProposalWritten{}, err
	}
	return crmcontracts.DealRoleProposalWritten{
		PersonId:         openapi_types.UUID(personID.UUID),
		FullName:         named[proposal.PersonID],
		Role:             proposal.Role,
		EvidenceSnippet:  proposal.EvidenceSnippet,
		SourceActivityId: openapi_types.UUID(mustParseActivity(proposal.SourceID)),
		Confidence:       float32(proposal.Confidence),
	}, nil
}

// recordEvidence lands the words each role was read from.
//
// The relationship row carries no evidence column, and adding one for this is a
// schema change the card does not need — but the evidence still has to outlive
// the HTTP response, or a reviewer can see that an agent seated somebody and
// never what it read. So it lands on an audit row against the DEAL: one entry
// per call, naming every seat, its quote and the message the quote came from.
func recordEvidence(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID,
	written []crmcontracts.DealRoleProposalWritten,
) error {
	seats := make([]map[string]any, 0, len(written))
	for _, seat := range written {
		seats = append(seats, map[string]any{
			"person_id":          seat.PersonId.String(),
			"role":               seat.Role,
			"evidence_snippet":   seat.EvidenceSnippet,
			"source_activity_id": seat.SourceActivityId.String(),
			"confidence":         seat.Confidence,
		})
	}
	// `assign` from the closed action vocabulary, because that is what this
	// did: it assigned buying roles. A verb of its own would need a migration
	// to widen the CHECK, and would buy nothing a reader could not already see
	// from the acting agent on the row.
	_, err := storekit.AuditEventWithEvidence(ctx, tx, "assign", "deal",
		dealID.UUID, map[string]any{"seats_written": len(written)},
		map[string]any{"seats": seats})
	if err != nil {
		return fmt.Errorf("org360: recording what the roles were read from: %w", err)
	}
	return nil
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

// seatedNow is who holds a seat on the deal at this moment, read UNSCOPED
// inside the writing transaction.
//
// Unscoped deliberately, and it is the one read here that is. The question is
// not "whose seats may this caller see" — it is "would this write second an
// answer somebody has already given", and a seat the caller cannot see is
// still an answer. Scoping it would let a reading overwrite exactly the seats
// its author was not allowed to know about. It discloses nothing: no name and
// no role leaves this function, only the decision not to write.
func seatedNow(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT person_id FROM relationship
		 WHERE kind = 'deal_stakeholder' AND deal_id = $1
		   AND archived_at IS NULL AND ended_at IS NULL
		   AND coalesce(role, '') <> ''`, dealID)
	if err != nil {
		return nil, fmt.Errorf("org360: re-reading the committee before writing: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var person ids.UUID
		if err := rows.Scan(&person); err != nil {
			return nil, err
		}
		out[person.String()] = true
	}
	return out, rows.Err()
}
