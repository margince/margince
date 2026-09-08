// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The two halves of a human-decided LinkedIn match (founder decision,
// 2026-08-02): the candidates that need deciding, and the write that happens
// when somebody says yes.
//
// The decision itself does NOT live here. It lives in the approvals engine,
// which is the product's one place where a proposal waits for a person — this
// module supplies the facts and performs the effect, and owns neither the
// queue nor the verdict.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The decided state a match lands in, and the person_social keys the handle
// write and its audit share. Named because SQL literals and Go comparisons of
// the same string are two places for one typo to orphan a link.
const (
	matchConfirmed = "confirmed"
	matchRejected  = "rejected"
	matchSuggested = "suggested"
	socialLinkedIn = "linkedin"
	auditKeySocial = "social"
)

// PendingLinkedInMatch is one candidate a human still has to judge, in the
// terms they judge it on: the export's own spelling of the connection, and the
// contact the matcher thinks it is.
type PendingLinkedInMatch struct {
	ConnectionID ids.UUID
	// OwnerUserID is the member whose imported network produced this pair. It
	// travels because the APPLY has to bind the connection it was staged for:
	// the guards on that path gate the person, and nothing tied the row.
	OwnerUserID       ids.UUID
	ConnectionName    string
	ConnectionCompany string
	PersonID          ids.UUID
	PersonName        string
}

// PendingLinkedInMatches lists the caller's suggested matches.
//
// `suggested` is the MATCHER's output, not a queue: it means "this pair is
// plausible and no string comparison can settle it". Turning each one into a
// proposal is the caller's job, and skipping the ones already decided is too —
// the approval row is the record of that, and this module does not read it.
func (s *Store) PendingLinkedInMatches(ctx context.Context) ([]PendingLinkedInMatch, error) {
	return s.suggestedMatches(ctx, ids.Nil)
}

// PendingLinkedInMatchesForPerson is the same list narrowed to ONE contact —
// what a caller that matched a single arrival owes a proposal pass over.
//
// The narrow read exists because the whole-network one is O(the member's open
// questions) and the caller runs once per person event: proposing every
// outstanding match again on each capture write only ever joins rows that
// already exist. Matching against one contact can raise questions about that
// contact and no other, so this is the complete answer for that caller as well
// as the cheap one.
func (s *Store) PendingLinkedInMatchesForPerson(ctx context.Context, personID ids.UUID) ([]PendingLinkedInMatch, error) {
	if personID == ids.Nil {
		// The zero id is exactly what the unfiltered read passes, so accepting
		// it here would silently widen a caller that meant to name one contact.
		return nil, errors.New("people: a person-scoped pending-match read was given no contact")
	}
	return s.suggestedMatches(ctx, personID)
}

// optionalPerson renders the contact filter the way SQL reads it: NULL for the
// unfiltered entry point, so the predicate below is one expression rather than
// two queries whose row-scope join would have to be kept in step by hand.
func optionalPerson(id ids.UUID) *ids.UUID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

// suggestedMatches is the one gated read both entry points land on. forPerson
// is ids.Nil for every contact.
func (s *Store) suggestedMatches(ctx context.Context, forPerson ids.UUID) ([]PendingLinkedInMatch, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	// The payload names a contact, so it takes the person read grant. Row scope
	// rides the join below.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []PendingLinkedInMatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		ownerPos := arg(actor.UserID)
		scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
		if err != nil {
			return err
		}
		visible := sqlAlwaysVisible
		if scope != "" {
			visible = scope
		}
		// NULL is every contact, so one query serves both entry points without a
		// second copy of the row-scope join to keep in step with this one.
		personPos := arg(optionalPerson(forPerson))
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT c.id, c.owner_user_id, c.full_name, coalesce(c.company_name, ''), p.id, p.full_name
			  FROM linkedin_connection c
			  JOIN person p ON p.id = c.matched_person_id AND p.archived_at IS NULL AND (%s)
			 WHERE c.owner_user_id = $%d
			   AND c.match_status = 'suggested'
			   AND c.tombstoned_at IS NULL
			   AND ($%d::uuid IS NULL OR c.matched_person_id = $%d::uuid)
			 ORDER BY c.full_name, c.id`, visible, ownerPos, personPos, personPos), args...)
		if err != nil {
			return fmt.Errorf("people: reading the LinkedIn matches awaiting a decision: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m PendingLinkedInMatch
			if err := rows.Scan(&m.ConnectionID, &m.OwnerUserID, &m.ConnectionName, &m.ConnectionCompany,
				&m.PersonID, &m.PersonName); err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// ApplyLinkedInMatch links a connection to a contact and puts the connection's
// LinkedIn address on that contact — the effect an approved proposal releases.
//
// It is the same write the automatic exact-name path performs. The difference
// is only who released it: a string comparison there, a person here.
func (s *Store) ApplyLinkedInMatch(ctx context.Context, connectionID, ownerID, personID ids.UUID) error {
	// Writing to a contact takes the person update grant. The approvals engine
	// checked the decider's authority before calling this; taking it again here
	// keeps the store's own entry point gated rather than trusting a caller.
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, entityPerson, personID); err != nil {
			return err
		}
		// And HELD, before the connection row below. person_social is a declared
		// PII table Art. 17 erasure deletes, so a handle written after that
		// commit puts the erased person's public profile straight back.
		//
		// Taken HERE rather than beside that write, because the erasure goes
		// person-then-linkedin_connection and this transaction locks the
		// connection two statements down. Person second would close a cycle
		// against it — the same ordering the DOI issuer takes, and for the same
		// reason.
		// The prior values come from the write itself, through a pre-write
		// self-join: a separate read would be a different look at the same row,
		// and the audit row would attest to something other than what this
		// statement replaced.
		var wasStatus string
		var wasPerson *ids.UUID
		// BOUND to the connection this proposal was staged for, on all three
		// axes the payload names. The guards above gate the PERSON — the update
		// grant and the target's visibility — and nothing tied the connection:
		// an apply would re-point an already-confirmed row to a different
		// contact, and would land on another member's connection, if the payload
		// said so.
		//
		// owner_user_id, because a member decides about their OWN imported
		// network and nobody else's. match_status = 'suggested', because a
		// confirmed row is a link somebody already has and this is not the verb
		// that moves one. matched_person_id, because the pair is the claim: an
		// approval released against this contact must not apply to whatever the
		// row points at now if the matcher moved it.
		err := tx.QueryRow(ctx, `
			UPDATE linkedin_connection c
			   SET match_status = 'confirmed', updated_at = now()
			  FROM linkedin_connection was
			 WHERE c.id = $1 AND was.id = c.id AND c.tombstoned_at IS NULL
			   AND c.owner_user_id = $3
			   AND c.match_status = 'suggested'
			   AND c.matched_person_id = $2
			 RETURNING was.match_status, was.matched_person_id`,
			connectionID, personID, ownerID).Scan(&wasStatus, &wasPerson)
		if errors.Is(err, pgx.ErrNoRows) {
			// Every way the predicate misses is the same answer, and it is the
			// honest one: this approval does not describe a suggestion that is
			// still there to confirm. The connection was tombstoned or erased,
			// or it belongs to another member, or it has already been confirmed,
			// or the pair it names has moved. Silently succeeding would report a
			// link that does not exist.
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("people: applying an approved LinkedIn match: %w", err)
		}
		wrote, err := writeLinkedInHandle(ctx, tx, connectionID, personID)
		if err != nil {
			return err
		}
		return auditLinkedInMatch(ctx, tx, connectionID, personID, matchImages(wasStatus, wasPerson, personID), wrote)
	})
}

// matchImagePair is the connection's own columns on either side of a confirmed
// match, narrowed to what actually moved: re-confirming a match that already
// names the same contact changes nothing, and an image saying otherwise would
// publish a decision the row did not record.
type matchImagePair struct{ before, after map[string]any }

func matchImages(wasStatus string, wasPerson *ids.UUID, personID ids.UUID) matchImagePair {
	// Dereferenced, because the two sides are compared by value: a *ids.UUID and
	// an ids.UUID never read as equal however they point, so a re-confirm of the
	// same contact would publish matched_person_id moving to what it already
	// held — which is the change this narrowing exists to leave out.
	var wasPersonValue any
	if wasPerson != nil {
		wasPersonValue = *wasPerson
	}
	before, after := storekit.ChangedColumns(
		map[string]any{"match_status": wasStatus, "matched_person_id": wasPersonValue},
		map[string]any{"match_status": matchConfirmed, "matched_person_id": personID},
	)
	return matchImagePair{before: before, after: after}
}

// auditLinkedInMatch commits the write shape. The connection's own audit row
// records the link; a handle that reached the contact is a second mutation of a
// second entity and takes its own audit and its own person.updated, so a trace
// consumer resolves each event to an audit of the entity it describes.
func auditLinkedInMatch(ctx context.Context, tx pgx.Tx, connectionID, personID ids.UUID, images matchImagePair, wroteURL bool) error {
	// Whether the profile URL reached the contact is context ABOUT this
	// decision, not a column on the connection, so it rides the evidence
	// column rather than the images field history projects.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "linkedin_connection", connectionID,
		images.before, images.after, map[string]any{"profile_url_written": wroteURL})
	if err != nil {
		return err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, connectionID,
		crmcontracts.PublicEventLinkedinMatchDecided{ProfileUrlWritten: wroteURL}); err != nil {
		return err
	}
	if !wroteURL {
		return nil
	}
	return auditLinkedInHandleGained(ctx, tx, personID)
}

// auditLinkedInHandleGained records that a contact's empty LinkedIn slot was
// filled, as its own audit row and its own person.updated.
//
// Both writers of person_social's linkedin slot land here — this decision and
// the research-claim slot fill — because a reader asking "when did this contact
// gain its profile link" must get the same answer whichever put it there. The
// slot fill in particular has no other way to say it: its caller's audit row
// describes the evidence write, and that row reads identically whether the slot
// was filled or was already occupied.
//
// The handle lands in person_social, never on a column of the person, so there
// is no field image to carry — what the contact gained is the whole of it.
func auditLinkedInHandleGained(ctx context.Context, tx pgx.Tx, personID ids.UUID) error {
	personAudit, err := storekit.AuditEvent(ctx, tx, "update", entityPerson, personID,
		map[string]any{auditKeySocial: []string{socialLinkedIn}})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, personAudit, personID,
		crmcontracts.PublicEventPersonUpdated{
			ChangedFields: map[string]any{auditKeySocial: []string{socialLinkedIn}},
		})
}

// writeLinkedInHandle stamps the member's LinkedIn profile URL onto the contact
// they just confirmed a connection to.
//
// This is the ONE thing a ghost contributes to a real record, and it is
// deliberately narrow: the URL and nothing else. The ghost's name, employer,
// position and connection date stay where they are — the export is a third
// party's data, and copying it onto a contact would be the consent problem the
// whole ghost design exists to avoid.
//
// The handle written is the CONNECTION's own profile URL — the `URL` column
// Connections.csv has always carried. NOT the member's own profile URL: that
// one belongs to the member, and stamping it on every contact they confirm
// would put the wrong person's address on the record.
//
// A connection imported before the URL column existed has no URL and writes nothing.
// The confirmation still stands; only the copy is unavailable, and the caller
// is told so rather than left to wonder.
//
// ON CONFLICT DO NOTHING: a handle already on the record is somebody's
// statement, and confirming a match is not grounds to replace it. The caller is
// told which happened rather than left to guess.
func writeLinkedInHandle(ctx context.Context, tx pgx.Tx, connectionID, personID ids.UUID) (bool, error) {
	var handle *string
	err := tx.QueryRow(ctx,
		`SELECT profile_url FROM linkedin_connection WHERE id = $1`, connectionID).Scan(&handle)
	if err != nil {
		return false, fmt.Errorf("people: reading a connection's profile URL: %w", err)
	}
	if handle == nil || *handle == "" {
		return false, nil
	}
	// No lock here: ApplyLinkedInMatch holds this subject from the top of its
	// transaction, ahead of the connection row, and re-taking it below that
	// would be the ordering the eraser deadlocks against. person_social is a
	// declared PII table Art. 17 deletes, and the hold is what stops a handle
	// landing after the erasure cleared it.
	_, landed, err := insertSocialHandle(ctx, tx, personID, socialLinkedIn, *handle)
	if err != nil || !landed {
		return false, err
	}
	return true, touchPerson(ctx, tx, personID)
}

// touchPerson bumps the person row so the aggregate's version moves with its
// children.
//
// person_social is part of the person aggregate, and the ordinary update path
// bumps the row for exactly this reason. Writing a child without it leaves a
// stale If-Match token valid: a browser holding version V would overwrite the
// social set it never saw, and replacePersonSocial replaces ALL rows, so the
// handle just written would vanish with no error anywhere.
//
// The row is LOCKED before the bump rather than updated blind. Two decisions
// landing on one contact at the same instant would otherwise both read the
// pre-bump version and one increment would be lost — the same TOCTOU shape
// every by-id update in this codebase is required to close.
func touchPerson(ctx context.Context, tx pgx.Tx, personID ids.UUID) error {
	if _, err := storekit.LockRow(ctx, tx, entityPerson, personID, storekit.LiveOnly); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE person SET updated_at = now() WHERE id = $1`, personID); err != nil {
		return fmt.Errorf("people: bumping the contact a LinkedIn handle changed: %w", err)
	}
	return nil
}

// RecordLinkedInMatchRefused writes the terminal state a refused suggestion has
// always had a value for and never a writer.
//
// The migration that created the column defines `rejected`; nothing in the tree
// wrote it. Rejecting a proposal runs no effect — the approvals engine
// dispatches its effect table on APPROVE only — so a refused connection stayed
// `suggested` and non-tombstoned for ever, and nothing reading the row could
// tell a finished suggestion from a live one.
//
// The refusal itself was never lost: StageUnlessDeclined reads the declined
// offers and will not re-propose. What was lost is the cost. The sweep
// enumerates every connection in (unmatched, suggested), so a refused one paid
// an RBAC resolve, a whole-network match, a pending read and a staging
// transaction on every hourly pass and every organization event, for a
// guaranteed-empty result — a permanent per-pass charge that grows with the
// size of the imported network, for a state that is supposed to be the cheap
// one.
//
// Written where the refusal is OBSERVED rather than where it is made: the
// stager already learns it, because StageUnlessDeclined answers "not staged"
// for exactly this reason. That keeps the whole change on this side of the
// seam — no reject-side effect table, which would be a new concept in the
// approvals engine for one caller — and it is self-healing: a connection
// refused before this shipped is marked the first time a sweep reaches it.
//
// Idempotent by predicate. A second pass finds the row already rejected, the
// UPDATE matches nothing, and no audit row is written for a decision nobody
// made twice.
func (s *Store) RecordLinkedInMatchRefused(ctx context.Context, connectionID ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return apperrors.ErrPermissionDenied
	}
	// person:read, the grant its SIBLING WRITER of this column takes.
	// MatchLinkedInConnections sets match_status to `suggested` under the read
	// grant, because what these rows are is one member's own imported network
	// and the authority over them is owning them. person:update is what
	// ApplyLinkedInMatch takes, and it takes it because it writes to a PERSON —
	// copying that here would refuse the whole staging pass for a ghost owner
	// holding read-only person grants, which is most of them.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var wasStatus string
		err := tx.QueryRow(ctx, `
			UPDATE linkedin_connection c
			   SET match_status = $2, updated_at = now()
			  FROM linkedin_connection was
			 WHERE c.id = $1 AND was.id = c.id AND c.tombstoned_at IS NULL
			   AND c.owner_user_id = $3
			   AND c.match_status = $4
			 RETURNING was.match_status`,
			connectionID, matchRejected, actor.UserID, matchSuggested).Scan(&wasStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			// Every way this matches nothing is a no-op and none is wrong.
			//
			// SUGGESTED and nothing else, which is the narrow predicate and the
			// one that matters: the refusal was observed against a snapshot, and
			// between that read and this write the row may have been confirmed
			// by the exact-name matcher or reset to unmatched by a re-import.
			// A refusal is only ever about the suggestion it answered, so a
			// stale observation must leave a newer state alone rather than
			// overwrite a link somebody now has.
			//
			// The owner clause is the same authority the read had: one member
			// marking another's connection is not a refusal they made.
			//
			// Already rejected, tombstoned or gone are the ordinary cases — the
			// sweep re-reaches a refused row on every pass until the
			// enumeration stops covering it, which is the point of it being
			// cheap to answer.
			return nil
		}
		if err != nil {
			return fmt.Errorf("people: recording a refused LinkedIn match: %w", err)
		}
		before, after := storekit.ChangedColumns(
			map[string]any{"match_status": wasStatus},
			map[string]any{"match_status": matchRejected})
		_, err = storekit.Audit(ctx, tx, "update", "linkedin_connection", connectionID, before, after)
		return err
	})
}
