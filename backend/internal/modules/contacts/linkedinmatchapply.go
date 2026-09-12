// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The two halves of a human-decided LinkedIn match (founder decision,
// 2026-08-02): the candidates that need deciding, and the write that happens
// when somebody says yes.
//
// The decision itself does NOT live here. It lives in the approvals engine,
// which is the product's one place where a proposal waits for a contact — this
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

// The decided state a match lands in, and the contact_social keys the handle
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
	// the guards on that path gate the contact, and nothing tied the row.
	OwnerUserID       ids.UUID
	ConnectionName    string
	ConnectionCompany string
	ContactID         ids.UUID
	ContactName       string
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

// PendingLinkedInMatchesForContact is the same list narrowed to ONE contact —
// what a caller that matched a single arrival owes a proposal pass over.
//
// The narrow read exists because the whole-network one is O(the member's open
// questions) and the caller runs once per contact event: proposing every
// outstanding match again on each capture write only ever joins rows that
// already exist. Matching against one contact can raise questions about that
// contact and no other, so this is the complete answer for that caller as well
// as the cheap one.
func (s *Store) PendingLinkedInMatchesForContact(ctx context.Context, contactID ids.UUID) ([]PendingLinkedInMatch, error) {
	if contactID == ids.Nil {
		// The zero id is exactly what the unfiltered read passes, so accepting
		// it here would silently widen a caller that meant to name one contact.
		return nil, errors.New("contacts: a contact-scoped pending-match read was given no contact")
	}
	return s.suggestedMatches(ctx, contactID)
}

// optionalContact renders the contact filter the way SQL reads it: NULL for the
// unfiltered entry point, so the predicate below is one expression rather than
// two queries whose row-scope join would have to be kept in step by hand.
func optionalContact(id ids.UUID) *ids.UUID {
	if id == ids.Nil {
		return nil
	}
	return &id
}

// suggestedMatches is the one gated read both entry points land on. forContact
// is ids.Nil for every contact.
func (s *Store) suggestedMatches(ctx context.Context, forContact ids.UUID) ([]PendingLinkedInMatch, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	// The payload names a contact, so it takes the contact read grant. Row scope
	// rides the join below.
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []PendingLinkedInMatch
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		ownerPos := arg(actor.UserID)
		scope, err := auth.ScopeClauseFor(ctx, "contact", "p", arg)
		if err != nil {
			return err
		}
		visible := sqlAlwaysVisible
		if scope != "" {
			visible = scope
		}
		// NULL is every contact, so one query serves both entry points without a
		// second copy of the row-scope join to keep in step with this one.
		contactPos := arg(optionalContact(forContact))
		rows, err := tx.Query(ctx, storekit.SQLf(`
			SELECT c.id, c.owner_user_id, c.full_name, coalesce(c.company_name, ''), p.id, p.full_name
			  FROM linkedin_connection c
			  JOIN contact p ON p.id = c.matched_contact_id AND p.archived_at IS NULL AND (%s)
			 WHERE c.owner_user_id = $%d
			   AND c.match_status = 'suggested'
			   AND c.tombstoned_at IS NULL
			   AND ($%d::uuid IS NULL OR c.matched_contact_id = $%d::uuid)
			 ORDER BY c.full_name, c.id`, visible, ownerPos, contactPos, contactPos), args...)
		if err != nil {
			return fmt.Errorf("contacts: reading the LinkedIn matches awaiting a decision: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m PendingLinkedInMatch
			if err := rows.Scan(&m.ConnectionID, &m.OwnerUserID, &m.ConnectionName, &m.ConnectionCompany,
				&m.ContactID, &m.ContactName); err != nil {
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
// is only who released it: a string comparison there, a contact here.
func (s *Store) ApplyLinkedInMatch(ctx context.Context, connectionID, ownerID, contactID ids.UUID) error {
	// Writing to a contact takes the contact update grant. The approvals engine
	// checked the decider's authority before calling this; taking it again here
	// keeps the store's own entry point gated rather than trusting a caller.
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return applyLinkedInMatchInTx(ctx, tx, connectionID, ownerID, contactID)
	})
}

// ApplyLinkedInMatchTx is ApplyLinkedInMatch inside a transaction the CALLER
// owns, for the accept effect.
//
// It exists so redeeming the approval and applying it are ONE transaction. They
// were two: the redemption committed, and a failure in the apply left the
// approval consumed and the connection never linked — with no way back through
// the API, because the row is decided and re-deciding answers 409. Redeem's own
// doc says callers should use RedeemAndApply and have no window at all, which
// every other accept effect in compose already does.
//
// The object gate is the caller's to apply, as it is for every other Tx-shaped
// entry point here: the approvals service checks the decider's authority before
// any effect runs, and ApplyLinkedInMatch applies it above for the direct path.
func ApplyLinkedInMatchTx(ctx context.Context, tx pgx.Tx, connectionID, ownerID, contactID ids.UUID) error {
	return applyLinkedInMatchInTx(ctx, tx, connectionID, ownerID, contactID)
}

// applyLinkedInMatchInTx is the write both entry points land on.
//
// A ZERO ownerID means the proposal predates the field, and it is resolved from
// the connection row rather than refused. A pending approval staged before this
// shipped carries no owner_user_id, so binding on the zero value would match no
// row — and the member could never decide it: the effect fails, and re-deciding
// a decided row answers 409. Reading the owner off the row is what staging does
// anyway, so a legacy payload gets exactly the behaviour it had, and a new one
// gets the guard. The fallback goes when no pending proposal predates the field.
func applyLinkedInMatchInTx(ctx context.Context, tx pgx.Tx, connectionID, ownerID, contactID ids.UUID) error {
	if ownerID == ids.Nil {
		if err := tx.QueryRow(ctx,
			`SELECT owner_user_id FROM linkedin_connection WHERE id = $1 AND tombstoned_at IS NULL`,
			connectionID).Scan(&ownerID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return fmt.Errorf("contacts: reading a legacy proposal's connection owner: %w", err)
		}
	}
	if err := auth.HoldWritableLive(ctx, tx, entityContact, contactID); err != nil {
		return err
	}
	// And HELD, before the connection row below. contact_social is a declared
	// PII table Art. 17 erasure deletes, so a handle written after that
	// commit puts the erased contact's public profile straight back.
	//
	// Taken HERE rather than beside that write, because the erasure goes
	// contact-then-linkedin_connection and this transaction locks the
	// connection two statements down. Contact second would close a cycle
	// against it — the same ordering the DOI issuer takes, and for the same
	// reason.
	// The prior values come from the write itself, through a pre-write
	// self-join: a separate read would be a different look at the same row,
	// and the audit row would attest to something other than what this
	// statement replaced.
	var wasStatus string
	var wasContact *ids.UUID
	// BOUND to the connection this proposal was staged for, on all three
	// axes the payload names. The guards above gate the CONTACT — the update
	// grant and the target's visibility — and nothing tied the connection:
	// an apply would re-point an already-confirmed row to a different
	// contact, and would land on another member's connection, if the payload
	// said so.
	//
	// owner_user_id, because a member decides about their OWN imported
	// network and nobody else's. match_status = 'suggested', because a
	// confirmed row is a link somebody already has and this is not the verb
	// that moves one. matched_contact_id, because the pair is the claim: an
	// approval released against this contact must not apply to whatever the
	// row points at now if the matcher moved it.
	err := tx.QueryRow(ctx, `
		UPDATE linkedin_connection c
		   SET match_status = 'confirmed', updated_at = now()
		  FROM linkedin_connection was
		 WHERE c.id = $1 AND was.id = c.id AND c.tombstoned_at IS NULL
		   AND c.owner_user_id = $3
		   AND c.match_status = 'suggested'
		   AND c.matched_contact_id = $2
		 RETURNING was.match_status, was.matched_contact_id`,
		connectionID, contactID, ownerID).Scan(&wasStatus, &wasContact)
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
		return fmt.Errorf("contacts: applying an approved LinkedIn match: %w", err)
	}
	wrote, err := writeLinkedInHandle(ctx, tx, connectionID, contactID)
	if err != nil {
		return err
	}
	return auditLinkedInMatch(ctx, tx, connectionID, ownerID, contactID, matchImages(wasStatus, wasContact, matchConfirmed, &contactID), wrote)
}

// matchImagePair is the connection's own columns on either side of a confirmed
// match, narrowed to what actually moved: re-confirming a match that already
// names the same contact changes nothing, and an image saying otherwise would
// publish a decision the row did not record.
type matchImagePair struct{ before, after map[string]any }

// matchImages builds the field image for either decision about a suggestion —
// the confirm and the refusal both call it. They move the same two columns, and
// a second builder that named those columns its own way would make one decision
// unreadable beside the other in the same field history.
func matchImages(wasStatus string, wasContact *ids.UUID, toStatus string, toContact *ids.UUID) matchImagePair {
	// Dereferenced on both sides, because they are compared by value: a
	// *ids.UUID and an ids.UUID never read as equal however they point, so a
	// re-confirm of the same contact would publish matched_contact_id moving to
	// what it already held — which is the change this narrowing exists to leave
	// out. A nil stays nil, which is how a refusal says the row now names
	// nobody.
	before, after := storekit.ChangedColumns(
		map[string]any{matchStatusColumn: wasStatus, matchedContactColumn: contactValue(wasContact)},
		map[string]any{matchStatusColumn: toStatus, matchedContactColumn: contactValue(toContact)},
	)
	return matchImagePair{before: before, after: after}
}

// contactValue is a matched_contact_id as the image comparison reads it.
//
//craft:ignore naked-any the field-image maps are map[string]any, and this feeds one — a typed return would be converted at the call site and say less
func contactValue(id *ids.UUID) any {
	if id == nil {
		return nil
	}
	return *id
}

// The two columns a decision about a suggestion moves. They are constants
// because both decisions write them and a field history is read across the two,
// so a typo on one side would silently split one column's history in half.
const (
	matchStatusColumn    = "match_status"
	matchedContactColumn = "matched_contact_id"
)

// auditLinkedInMatch commits the write shape. The connection's own audit row
// records the link; a handle that reached the contact is a second mutation of a
// second entity and takes its own audit and its own contact.updated, so a trace
// consumer resolves each event to an audit of the entity it describes.
func auditLinkedInMatch(ctx context.Context, tx pgx.Tx, connectionID, ownerID, contactID ids.UUID, images matchImagePair, wroteURL bool) error {
	// Whether the profile URL reached the contact is context ABOUT this
	// decision, not a column on the connection, so it rides the evidence
	// column rather than the images field history projects.
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "linkedin_connection", connectionID,
		images.before, images.after, map[string]any{"profile_url_written": wroteURL})
	if err != nil {
		return err
	}
	// The event's subject is the MEMBER whose network produced the match; the
	// audit row above names the connection because that is the row this
	// statement changed. Two different questions, and they had one answer by
	// accident: the event carried the CONNECTION id under a declared entity
	// type of `user`.
	//
	// That made it undeliverable, always. linkedin_match.decided is a self-only
	// event — a colleague's professional network is theirs — so delivery admits
	// it only when the subscriber IS the user the id names, and a connection
	// uuid can never equal a user id. The check could not pass for anybody.
	//
	// ownerID is the owner the write above already bound on, so the event names
	// the same member the statement was predicated on rather than a second
	// read's answer.
	if err := storekit.EmitEvent(ctx, tx, auditID, ownerID,
		crmcontracts.PublicEventLinkedinMatchDecided{ProfileUrlWritten: wroteURL}); err != nil {
		return err
	}
	if !wroteURL {
		return nil
	}
	return auditLinkedInHandleGained(ctx, tx, contactID)
}

// auditLinkedInHandleGained records that a contact's empty LinkedIn slot was
// filled, as its own audit row and its own contact.updated.
//
// Both writers of contact_social's linkedin slot land here — this decision and
// the research-claim slot fill — because a reader asking "when did this contact
// gain its profile link" must get the same answer whichever put it there. The
// slot fill in particular has no other way to say it: its caller's audit row
// describes the evidence write, and that row reads identically whether the slot
// was filled or was already occupied.
//
// The handle lands in contact_social, never on a column of the contact, so there
// is no field image to carry — what the contact gained is the whole of it.
func auditLinkedInHandleGained(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	contactAudit, err := storekit.AuditEvent(ctx, tx, "update", entityContact, contactID,
		map[string]any{auditKeySocial: []string{socialLinkedIn}})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, contactAudit, contactID,
		crmcontracts.PublicEventContactUpdated{
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
// would put the wrong contact's address on the record.
//
// A connection imported before the URL column existed has no URL and writes nothing.
// The confirmation still stands; only the copy is unavailable, and the caller
// is told so rather than left to wonder.
//
// ON CONFLICT DO NOTHING: a handle already on the record is somebody's
// statement, and confirming a match is not grounds to replace it. The caller is
// told which happened rather than left to guess.
func writeLinkedInHandle(ctx context.Context, tx pgx.Tx, connectionID, contactID ids.UUID) (bool, error) {
	var handle *string
	err := tx.QueryRow(ctx,
		`SELECT profile_url FROM linkedin_connection WHERE id = $1`, connectionID).Scan(&handle)
	if err != nil {
		return false, fmt.Errorf("contacts: reading a connection's profile URL: %w", err)
	}
	if handle == nil || *handle == "" {
		return false, nil
	}
	// No lock here: ApplyLinkedInMatch holds this subject from the top of its
	// transaction, ahead of the connection row, and re-taking it below that
	// would be the ordering the eraser deadlocks against. contact_social is a
	// declared PII table Art. 17 deletes, and the hold is what stops a handle
	// landing after the erasure cleared it.
	_, landed, err := insertSocialHandle(ctx, tx, contactID, socialLinkedIn, *handle)
	if err != nil || !landed {
		return false, err
	}
	return true, touchContact(ctx, tx, contactID)
}

// touchContact bumps the contact row so the aggregate's version moves with its
// children.
//
// contact_social is part of the contact aggregate, and the ordinary update path
// bumps the row for exactly this reason. Writing a child without it leaves a
// stale If-Match token valid: a browser holding version V would overwrite the
// social set it never saw, and replaceContactSocial replaces ALL rows, so the
// handle just written would vanish with no error anywhere.
//
// The row is LOCKED before the bump rather than updated blind. Two decisions
// landing on one contact at the same instant would otherwise both read the
// pre-bump version and one increment would be lost — the same TOCTOU shape
// every by-id update in this codebase is required to close.
func touchContact(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	if _, err := storekit.LockRow(ctx, tx, entityContact, contactID, storekit.LiveOnly); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contact SET updated_at = now() WHERE id = $1`, contactID); err != nil {
		return fmt.Errorf("contacts: bumping the contact a LinkedIn handle changed: %w", err)
	}
	return nil
}
