// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What CHANGED about a contact's relationship, gathered for the pure
// derivation in shared/kernel/relstrength.
//
// No table backs this and none should. The arithmetic that produces the
// current score is a fold over the contact's own interactions, so folding the
// same curve over a window that ends in the past recovers what the score WAS —
// and the difference is the change. A stored change log would be a second copy
// of a fact the activities already carry, with its own erasure surface, its own
// replay-safety question and its own way of disagreeing with the timeline
// beside it.
//
// The cost is one extra pass over one contact's interactions, which is the same
// order as the timeline query rendered next to it.
//
// Both passes carry auth.AudienceWorkspaceOnly, for the reason stated where it
// is defined: a change derived from a held message discloses that message. The
// score this differences is folded in strength.go, which asks the same question
// of the same kinds — a change computed over a wider set than the score it
// reports a change IN would also be arithmetic about two different populations.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// ContactRelationshipChangesTx reports what has happened to this contact's
// relationship, inside a caller's transaction.
//
// Row-scoped exactly like the score it explains: a contact the caller cannot
// read yields a not-found, never an empty list, because an empty list is a
// claim about a record whose existence is being hidden.
func (s *Store) ContactRelationshipChangesTx(
	ctx context.Context,
	tx pgx.Tx,
	contactID ids.ContactID,
	now time.Time,
	within *ids.ProjectID,
) ([]relstrength.Change, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
		return nil, err
	}
	in, err := changeInputs(ctx, tx, contactID, now, within)
	if err != nil {
		return nil, err
	}
	return relstrength.Changes(in, now), nil
}

// ContactChanges is one contact's derived changes, carrying the id and the name
// so a batched caller can say whose they are without a second read.
type ContactChanges struct {
	ContactID   ids.ContactID
	DisplayName string
	Changes     []relstrength.Change
}

// RelationshipChangesForContacts derives changes for a SET of contacts.
//
// The per-contact reader above probes the one contact it was handed, because
// that id came off a request. This one filters the whole set through the
// caller's row scope in a single pass instead — same rule, one query, and a
// contact the caller cannot read is simply absent rather than 404-ing a lane
// that is about somebody else's relationships too.
//
// It is bounded on purpose. `change.go` states that the derivation cannot
// answer "everyone who went cold" without walking every contact, and that stays
// true — what makes this affordable is that the caller narrows to a capped
// candidate set FIRST and derives only those. Handing it the workspace would be
// the walk that comment warns against.
func (s *Store) RelationshipChangesForContacts(
	ctx context.Context,
	tx pgx.Tx,
	contacts []ids.ContactID,
	now time.Time,
) ([]ContactChanges, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if len(contacts) == 0 {
		return nil, nil
	}
	visible, err := visibleContactNames(ctx, tx, contacts)
	if err != nil {
		return nil, err
	}
	out := make([]ContactChanges, 0, len(visible))
	for _, contact := range visible {
		in, err := changeInputs(ctx, tx, contact.id, now, nil)
		if err != nil {
			return nil, err
		}
		changes := relstrength.Changes(in, now)
		if len(changes) == 0 {
			continue
		}
		out = append(out, ContactChanges{
			ContactID:   contact.id,
			DisplayName: contact.name,
			Changes:     changes,
		})
	}
	return out, nil
}

// contactName is one readable contact: who they are, and what to call them.
type contactName struct {
	id   ids.ContactID
	name string
}

// visibleContactNames narrows a candidate set to the contacts this caller may
// actually read, and names them in the same pass.
//
// One query rather than a probe each: the row scope is a predicate, so asking
// it once for the set is the same rule the per-contact path applies one at a
// time. Archived contacts are excluded here exactly as every ordinary contact
// read excludes them.
func visibleContactNames(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID) ([]contactName, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos := arg(contacts)
	scope, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT p.id, p.full_name FROM contact p
		WHERE p.id = ANY($%d) AND p.archived_at IS NULL AND (%s)
		ORDER BY p.id`, contactsPos, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a contact set this caller may see: %w", err)
	}
	defer rows.Close()
	var out []contactName
	for rows.Next() {
		var contact contactName
		if err := rows.Scan(&contact.id, &contact.name); err != nil {
			return nil, err
		}
		out = append(out, contact)
	}
	return out, rows.Err()
}

// changeInputs gathers the present fold, the same fold as it stood one
// comparison window ago, and the two timestamps a returning reply is measured
// between.
func changeInputs(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	now time.Time, within *ids.ProjectID,
) (relstrength.ChangeInputs, error) {
	asOf := now.AddDate(0, 0, -relstrength.ComparisonDays)

	// BOTH windows and both statements, or the comparison is between two
	// different questions: the "before" fold would cover every project while
	// the "after" covered one, and the change it reported would be the
	// narrowing rather than anything that happened.
	foldArgs := []any{
		contactID,
		now.AddDate(0, 0, -relStrengthWindowDays),
		asOf,
		asOf.AddDate(0, 0, -relStrengthWindowDays),
	}
	foldWithin := ""
	if within != nil {
		foldArgs = append(foldArgs, *within)
		foldWithin = " AND " + activityWithinProject("a", len(foldArgs))
	}

	var in relstrength.ChangeInputs
	// One pass over the contact's qualifying interactions answers both windows.
	// The earlier fold's own last-interaction is the last one BEFORE asOf, not
	// the overall one — inheriting today's recency would make every band look
	// unchanged and the whole derivation silent.
	if err := tx.QueryRow(ctx, `
		SELECT max(a.occurred_at),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'inbound'),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $2 AND a.direction = 'outbound'),
		       max(a.occurred_at) FILTER (WHERE a.occurred_at < $3),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $4 AND a.occurred_at < $3),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $4 AND a.occurred_at < $3 AND a.direction = 'inbound'),
		       count(DISTINCT `+strengthInteractionUnit+`) FILTER (WHERE a.occurred_at >= $4 AND a.occurred_at < $3 AND a.direction = 'outbound'),
		       max(a.occurred_at) FILTER (WHERE a.direction = 'inbound')
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $1
		 WHERE a.kind IN `+strengthKinds+` AND a.archived_at IS NULL`+
		auth.AudienceWorkspaceOnly("a")+foldWithin,
		foldArgs...,
	).Scan(
		&in.Current.LastInteraction, &in.Current.Count90d, &in.Current.Inbound90d, &in.Current.Outbound90d,
		&in.Previous.LastInteraction, &in.Previous.Count90d, &in.Previous.Inbound90d, &in.Previous.Outbound90d,
		&in.LatestInbound,
	); err != nil {
		return relstrength.ChangeInputs{}, fmt.Errorf("contacts: reading a contact's relationship history: %w", err)
	}

	if in.LatestInbound == nil {
		return in, nil
	}
	// The far side of the silence their reply broke. A separate statement
	// because it is bounded by a value the first one returns.
	precedingArgs := []any{contactID, *in.LatestInbound}
	precedingWithin := ""
	if within != nil {
		precedingArgs = append(precedingArgs, *within)
		precedingWithin = " AND " + activityWithinProject("a", len(precedingArgs))
	}
	if err := tx.QueryRow(ctx, `
		SELECT max(a.occurred_at)
		  FROM activity a
		  JOIN activity_link l ON l.activity_id = a.id AND l.contact_id = $1
		 WHERE a.kind IN `+strengthKinds+` AND a.archived_at IS NULL
		   AND a.occurred_at < $2`+auth.AudienceWorkspaceOnly("a")+precedingWithin,
		precedingArgs...,
	).Scan(&in.PrecedingInteraction); err != nil {
		return relstrength.ChangeInputs{}, fmt.Errorf("contacts: reading what preceded a contact's last reply: %w", err)
	}
	return in, nil
}
