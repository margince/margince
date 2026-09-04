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
		until := time.Now().UTC().AddDate(0, 0, days)
		if _, err := tx.Exec(ctx, `
			INSERT INTO relationship_nudge_dismissal
			       (person_id, reader_id, dismissed_until, set_by, set_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (person_id, reader_id) DO UPDATE
			   SET dismissed_until = EXCLUDED.dismissed_until,
			       set_by = EXCLUDED.set_by,
			       set_at = now()`,
			personID, reader, until, by); err != nil {
			return fmt.Errorf("people: dismissing a relationship nudge: %w", err)
		}
		return recordNudgeDismissal(ctx, tx, personID, actionDismissed, &until)
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
		tag, err := tx.Exec(ctx, `
			DELETE FROM relationship_nudge_dismissal
			 WHERE person_id = $1 AND reader_id = $2`, personID, reader)
		if err != nil {
			return fmt.Errorf("people: restoring a relationship nudge: %w", err)
		}
		// Audited only when a row actually went. A restore of a contact who was
		// never set aside changed nothing, and an audit row for it would put a
		// judgement in the trail that nobody made.
		if tag.RowsAffected() == 0 {
			return nil
		}
		return recordNudgeDismissal(ctx, tx, personID, actionRestored, nil)
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
	ctx context.Context, tx pgx.Tx, personID ids.PersonID, action string, until *time.Time,
) error {
	after := map[string]any{"nudge_dismissal": action}
	if until != nil {
		after["dismissed_until"] = until.UTC()
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", "person", personID.UUID, after)
	if err != nil {
		return err
	}
	payload := crmcontracts.PublicEventRelationshipNudgeDismissed{
		PersonId: openapi_types.UUID(personID.UUID),
		Action:   crmcontracts.PublicEventRelationshipNudgeDismissedAction(action),
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

// DismissedNudges is which of these contacts THIS reader has set aside, at the
// given instant.
//
// Expiry is applied in the QUERY rather than by a sweep: a dismissal that has
// run out is simply not returned, so there is no job whose lateness would keep
// a contact hidden past the moment the rep chose.
func (s *Store) DismissedNudges(
	ctx context.Context, tx pgx.Tx, people []ids.PersonID, at time.Time,
) (map[ids.UUID]bool, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	out := map[ids.UUID]bool{}
	if len(people) == 0 {
		return out, nil
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return nil, err
	}
	ids_ := make([]ids.UUID, 0, len(people))
	for _, p := range people {
		ids_ = append(ids_, p.UUID)
	}
	rows, err := tx.Query(ctx, `
		SELECT person_id FROM relationship_nudge_dismissal
		 WHERE reader_id = $1 AND person_id = ANY($2) AND dismissed_until > $3`,
		reader, ids_, at)
	if err != nil {
		return nil, fmt.Errorf("people: reading nudge dismissals: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("people: reading nudge dismissals: %w", err)
		}
		out[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading nudge dismissals: %w", err)
	}
	return out, nil
}
