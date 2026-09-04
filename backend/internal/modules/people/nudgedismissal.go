// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// A lapsed contact one reader has set aside.
//
// The decay lane names contacts who have gone quiet. Nobody is waiting on the
// reader for any of them, which is why they go unnoticed — and why a rep needs
// a way to say "not this one, not now" without the row coming back tomorrow.
//
// PER READER, like a message's snooze and for the same reason: a rep deciding
// not to chase somebody this month is judging their own morning, and applying
// that to a colleague would take a contact off a queue whose owner never made
// the call.
//
// NEVER PERMANENT. The column is NOT NULL, so there is no forever to store. A
// permanent dismissal would silently delete a person from a rep's attention and
// leave nothing to notice it, which is the failure the hidden-backlog guardrail
// exists to catch — reached by a door that guardrail cannot see.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// nudgeDismissalMaxDays bounds how long a contact may be set aside.
//
// A quarter is the longest a rep can honestly say "not this one" about a
// relationship without that being a decision to drop the person, which is a
// different act with its own record. Spelled here and in the contract's
// `maximum`, and the contract is what a client is promised — this is the
// refusal that holds when a caller ignores it.
const nudgeDismissalMaxDays = 90

// DismissRelationshipNudge sets a lapsed contact aside for this reader.
//
// Re-dismissing replaces the moment rather than refusing: a rep who set someone
// aside for a week and then wants a month is saying one thing, not two.
func (s *Store) DismissRelationshipNudge(
	ctx context.Context, personID ids.PersonID, days int,
) error {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return err
	}
	if days < 1 || days > nudgeDismissalMaxDays {
		return fmt.Errorf(
			"people: a nudge dismissal runs 1 to %d days, not %d: %w",
			nudgeDismissalMaxDays, days, apperrors.ErrInvalidArgument)
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// Reading the person is the licence to set them aside, and a contact
		// this reader cannot open answers 404 like one that does not exist.
		if err := auth.EnsureVisible(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// The lock every row naming a person takes before it is written. An
		// archive in flight can otherwise commit between its own probe and its
		// sweep, leaving a live dismissal on an archived contact — which reads
		// as fine to every sequential test.
		if err := lockPersonForAttach(ctx, tx, personID); err != nil {
			return err
		}
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		// The moment is computed HERE rather than taken from the caller: the
		// server owns now, and a client computing an instant from a clock that
		// is minutes out writes a dismissal that expires early or late for a
		// reason nobody can see.
		//
		// time.Now() rather than an injected clock, matching every other write
		// in this store. The tests below assert the SPAN the row carries rather
		// than an instant, so nothing here needs a clock held still.
		//
		// TRUNCATED TO THE MICROSECOND, because that is all a timestamptz holds.
		// This value goes into the audit row's after-image as well as into the
		// column, and the NEXT dismissal images what it displaced by reading the
		// column back — so an image carrying nanoseconds names an instant the
		// row never held, and the two images of one deadline stop matching.
		//
		// The precision of time.Now() is the platform's: nanoseconds on Linux,
		// microseconds on Darwin. Without this the trail is right on one
		// developer's machine and wrong in CI, which is how it got here.
		until := time.Now().UTC().Truncate(time.Microsecond).AddDate(0, 0, days)
		// What deadline was already standing, read under the person lock taken
		// above so the upsert below cannot race it. A re-dismissal REPLACES this
		// value, and an audit row that named only the new one would leave
		// nobody able to say what it displaced — which is the before-image every
		// audited replacement owes.
		var replaced *time.Time
		if err := tx.QueryRow(ctx, `
			SELECT dismissed_until FROM relationship_nudge_dismissal
			 WHERE person_id = $1 AND reader_id = $2`,
			personID, reader).Scan(&replaced); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("people: reading the standing dismissal: %w", err)
		}
		// Every placeholder derived from the argument slice rather than typed:
		// nothing checks that a hand-written $N still names the value a caller
		// appends, and this statement carries five.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO relationship_nudge_dismissal
			       (person_id, reader_id, dismissed_until, set_by, set_at)
			VALUES ($%[1]d, $%[2]d, $%[3]d, $%[4]d, now())
			ON CONFLICT (person_id, reader_id) DO UPDATE
			   SET dismissed_until = EXCLUDED.dismissed_until,
			       set_by = EXCLUDED.set_by,
			       set_at = now()`,
			arg(personID), arg(reader), arg(until), arg(by)),
			args...); err != nil {
			return fmt.Errorf("people: dismissing a relationship nudge: %w", err)
		}
		return recordNudgeDismissal(ctx, tx, personID, actionDismissed, &until, replaced)
	})
}

// RestoreRelationshipNudge puts a set-aside contact back on this reader's lane.
//
// Idempotent: restoring a contact nobody dismissed is the same success, because
// the reader's goal state already holds.
func (s *Store) RestoreRelationshipNudge(ctx context.Context, personID ids.PersonID) error {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return err
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "person", personID.UUID); err != nil {
			return err
		}
		// The same lock the dismissal takes, and it is not symmetry for its own
		// sake. EnsureVisible returns without querying for an actor the row
		// scope does not bound, so on its own this path would answer 204 for a
		// person id that does not exist and would audit against an archived
		// one — while a scoped caller got 404 for the same row, which is itself
		// a way to tell an archived contact from a missing one.
		if err := lockPersonForAttach(ctx, tx, personID); err != nil {
			return err
		}
		// The row goes whether or not it had lapsed — there is no sweep, so a
		// stale one would otherwise sit there forever. What decides whether this
		// is a JUDGEMENT is whether the dismissal was still applying: restoring
		// a contact who was already back on the lane changed nothing anybody can
		// see, and announcing it would report a decision nobody made.
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		var wasApplying bool
		err := tx.QueryRow(ctx, fmt.Sprintf(`
			DELETE FROM relationship_nudge_dismissal
			 WHERE person_id = $%[1]d AND reader_id = $%[2]d
			 RETURNING dismissed_until > now()`,
			arg(personID), arg(reader)), args...).Scan(&wasApplying)
		if errors.Is(err, pgx.ErrNoRows) {
			// Never set aside. Idempotent: the reader's goal state holds.
			return nil
		}
		if err != nil {
			return fmt.Errorf("people: restoring a relationship nudge: %w", err)
		}
		if !wasApplying {
			return nil
		}
		return recordNudgeDismissal(ctx, tx, personID, actionRestored, nil, nil)
	})
}

// The two verbs, as the event spells them.
const (
	actionDismissed = "dismissed"
	actionRestored  = "restored"
)

// recordNudgeDismissal is the write shape's second half: the audit row and the
// announcement, in the same transaction as the judgement itself.
func recordNudgeDismissal(
	ctx context.Context, tx pgx.Tx, personID ids.PersonID,
	action string, until, replaced *time.Time,
) error {
	after := map[string]any{"nudge_dismissal": action}
	if until != nil {
		after["dismissed_until"] = until.UTC()
	}
	// A re-dismissal replaces a deadline that was standing, so it images the
	// one it displaced. A first dismissal replaces nothing and records an
	// occurrence — which is the difference between the two, spelled by whether
	// there was a prior row rather than by which verb was pressed.
	var auditID ids.UUID
	var err error
	if replaced != nil {
		before := map[string]any{"dismissed_until": replaced.UTC()}
		auditID, err = storekit.Audit(ctx, tx, "update", "person", personID.UUID, before, after)
	} else {
		auditID, err = storekit.AuditEvent(ctx, tx, "update", "person", personID.UUID, after)
	}
	if err != nil {
		return err
	}
	payload := crmcontracts.PublicEventRelationshipNudgeDecided{
		PersonId: openapi_types.UUID(personID.UUID),
		Action:   crmcontracts.PublicEventRelationshipNudgeDecidedAction(action),
	}
	if until != nil {
		moment := until.UTC()
		payload.DismissedUntil = &moment
	}
	return storekit.EmitEvent(ctx, tx, auditID, personID.UUID, payload)
}

// humanReader is whose morning this judgement is about.
//
// A dismissal binds ONE reader, so a principal with no human behind it has no
// morning to judge — refused rather than written against a zero id, which would
// be a row every agent shared.
func humanReader(ctx context.Context) (ids.UUID, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

// NotDismissedClause renders the predicate that keeps a contact ON the lane.
//
// Supplied to the candidate projection so it applies BEFORE that read's cap:
// the projection takes the oldest silences up to a bound, and a contact
// filtered after the cut has already spent a slot — so a rep who set several
// aside would lose real lapses off the bottom of their lane with nothing to say
// so. The waiting queue applies its own set-aside rule before its cap for the
// same reason, and says so in as many words.
//
// The alias names the edge row in the caller's statement. A reader with no
// human behind it is refused rather than given a permissive clause: this
// answers "which of MY contacts", and there is no my.
func NotDismissedClause(ctx context.Context, alias string, at time.Time, arg func(any) int) (string, error) {
	reader, err := humanReader(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`NOT EXISTS (
		SELECT 1 FROM relationship_nudge_dismissal d
		 WHERE d.person_id = %s.person_id
		   AND d.reader_id = $%d
		   AND d.dismissed_until > $%d)`, alias, arg(reader), arg(at)), nil
}
