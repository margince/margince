// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

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
// Temporary or permanent. A dated row sets the contact aside until a moment. A
// NULL `dismissed_until` is a stop with no end date, for a reader who has
// decided the relationship is over. Both stay reversible through
// RestoreRelationshipNudge, and neither hides the contact's own page.

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
// relationship without that being a decision to drop the contact, which is a
// different act with its own record. Spelled here and in the contract's
// `maximum`, and the contract is what a client is promised — this is the
// refusal that holds when a caller ignores it.
const nudgeDismissalMaxDays = 90

// NudgeDaysError is the 422 for a span outside 1 to nudgeDismissalMaxDays, the
// range the contract publishes as the field's minimum and maximum.
type NudgeDaysError struct{ Days int }

func (e *NudgeDaysError) Error() string {
	return fmt.Sprintf("a nudge dismissal runs 1 to %d days, not %d", nudgeDismissalMaxDays, e.Days)
}

// FieldFault names the request field for httperr's 422.
func (e *NudgeDaysError) FieldFault() (field, code, message string) {
	return "days", "out_of_range", e.Error()
}

// DismissRelationshipNudge sets a lapsed contact aside for this reader.
//
// Re-dismissing replaces the moment rather than refusing: a rep who set someone
// aside for a week and then wants a month is saying one thing, not two.
func (s *Store) DismissRelationshipNudge(
	ctx context.Context, contactID ids.ContactID, days int,
) error {
	return s.DismissRelationshipNudges(ctx, []ids.ContactID{contactID}, NudgeSpan{Days: days})
}

// NudgeSpan is how long a dismissal holds: Days from now, or Permanent.
type NudgeSpan struct {
	Days      int
	Permanent bool
}

// until is the stored end of the span: nil for a permanent stop.
func (span NudgeSpan) until(now time.Time) (*time.Time, error) {
	if span.Permanent {
		if span.Days != 0 {
			return nil, &NudgeDaysError{Days: span.Days}
		}
		return nil, nil //nolint:nilnil // a permanent stop stores no end moment, and NULL is that stop
	}
	if span.Days < 1 || span.Days > nudgeDismissalMaxDays {
		return nil, &NudgeDaysError{Days: span.Days}
	}
	moment := dismissalDeadline(now, span.Days)
	return &moment, nil
}

// DismissRelationshipNudges sets every listed contact aside for this reader,
// in one transaction: all of them or none. A contact the reader cannot read
// refuses the whole request, as it does for one.
func (s *Store) DismissRelationshipNudges(
	ctx context.Context, contactIDs []ids.ContactID, span NudgeSpan,
) error {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	now := s.clock()
	until, err := span.until(now)
	if err != nil {
		return err
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		for _, contactID := range contactIDs {
			if err := setNudgeAside(ctx, tx, contactID, reader, now, until); err != nil {
				return err
			}
		}
		return nil
	})
}

// setNudgeAside writes one reader's dismissal of one contact, with its audit
// and event, after the same visibility check a single dismissal makes.
func setNudgeAside(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, reader ids.UUID, now time.Time, until *time.Time) error {
	if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
		return err
	}
	if err := lockContactForAttach(ctx, tx, contactID); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	var replaced *time.Time
	found := true
	if err := tx.QueryRow(ctx, `
		SELECT dismissed_until FROM relationship_nudge_dismissal
		 WHERE contact_id = $1 AND reader_id = $2`,
		contactID, reader).Scan(&replaced); errors.Is(err, pgx.ErrNoRows) {
		found = false
	} else if err != nil {
		return fmt.Errorf("contacts: reading the standing dismissal: %w", err)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := fmt.Sprintf(`
		INSERT INTO relationship_nudge_dismissal
		       (contact_id, reader_id, dismissed_until, set_by, set_at)
		VALUES ($%[1]d, $%[2]d, $%[3]d, $%[4]d, $%[5]d)
		ON CONFLICT (contact_id, reader_id) DO UPDATE
		   SET dismissed_until = EXCLUDED.dismissed_until,
		       set_by = EXCLUDED.set_by,
		       set_at = EXCLUDED.set_at`,
		arg(contactID), arg(reader), arg(until), arg(by), arg(now.UTC().Truncate(time.Microsecond)))
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("contacts: dismissing a relationship nudge: %w", err)
	}
	return recordNudgeDismissal(ctx, tx, contactID, actionDismissed, until, found, replaced)
}

// NudgeState is one reader's standing choice about a quiet contact.
type NudgeState struct {
	// Stopped is a permanent stop; Until is set for a dated dismissal still in
	// force. Neither means the reader follows the contact.
	Stopped bool
	Until   *time.Time
}

// RelationshipNudgeState reads this reader's own choice for the contact, under
// the same visibility check a dismissal makes. A colleague's choice is never
// read.
func (s *Store) RelationshipNudgeState(ctx context.Context, contactID ids.ContactID) (NudgeState, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return NudgeState{}, err
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return NudgeState{}, err
	}
	var state NudgeState
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		var until *time.Time
		err := tx.QueryRow(ctx, `
			SELECT dismissed_until FROM relationship_nudge_dismissal
			 WHERE contact_id = $1 AND reader_id = $2`, contactID, reader).Scan(&until)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("contacts: reading the reader's nudge choice: %w", err)
		}
		switch {
		case until == nil:
			state.Stopped = true
		case until.After(s.clock()):
			state.Until = until
		}
		return nil
	})
	return state, err
}

// RestoreRelationshipNudge puts a set-aside contact back on this reader's lane.
//
// Idempotent: restoring a contact nobody dismissed is the same success, because
// the reader's goal state already holds.
func (s *Store) RestoreRelationshipNudge(ctx context.Context, contactID ids.ContactID) error {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return err
	}
	reader, err := humanReader(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		// The same lock the dismissal takes, and it is not symmetry for its own
		// sake. EnsureVisible returns without querying for an actor the row
		// scope does not bound, so on its own this path would answer 204 for a
		// contact id that does not exist and would audit against an archived
		// one — while a scoped caller got 404 for the same row, which is itself
		// a way to tell an archived contact from a missing one.
		if err := lockContactForAttach(ctx, tx, contactID); err != nil {
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
		query := fmt.Sprintf(`
			DELETE FROM relationship_nudge_dismissal
			 WHERE contact_id = $%[1]d AND reader_id = $%[2]d
			 RETURNING dismissed_until IS NULL OR dismissed_until > $%[3]d`,
			arg(contactID), arg(reader), arg(s.clock()))
		err := tx.QueryRow(ctx, query, args...).Scan(&wasApplying)
		if errors.Is(err, pgx.ErrNoRows) {
			// Never set aside. Idempotent: the reader's goal state holds.
			return nil
		}
		if err != nil {
			return fmt.Errorf("contacts: restoring a relationship nudge: %w", err)
		}
		if !wasApplying {
			return nil
		}
		return recordNudgeDismissal(ctx, tx, contactID, actionRestored, nil, false, nil)
	})
}

// The two verbs, as the event spells them.
const (
	actionDismissed = "dismissed"
	actionRestored  = "restored"
)

// recordNudgeDismissal is the write shape's second half: the audit row and the
// dismissalDeadline is when a dismissal lapses, at the precision the column
// can actually hold.
//
// TRUNCATED TO MICROSECONDS, which is what makes the audit trail honest rather
// than merely tidy. `timestamptz` stores microseconds, so a Go instant carries
// more precision than the row ever will — and the two images of one deadline
// are read back from different places: a dismissal audits the value it just
// computed, while a RE-dismissal images the deadline it displaced by reading
// the column. Untruncated, the second audit's before-image and the first
// audit's after-image describe one instant in two precisions, and a reader
// diffing the trail sees a change that never happened.
//
// Invisible on a host whose clock is already microsecond-granular, which is
// why it survived: the same sequence agrees on macOS and disagrees on the
// Linux runners, so it read as a flaky test rather than as a trail that does
// not join up.
func dismissalDeadline(now time.Time, days int) time.Time {
	return now.UTC().AddDate(0, 0, days).Truncate(time.Microsecond)
}

// announcement, in the same transaction as the judgement itself.
func recordNudgeDismissal(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID,
	action string, until *time.Time, hadOne bool, replaced *time.Time,
) error {
	after := map[string]any{"nudge_dismissal": action}
	permanent := action == actionDismissed && until == nil
	if until != nil {
		after["dismissed_until"] = until.UTC()
	}
	if permanent {
		after["permanent"] = true
	}
	// A re-dismissal replaces a deadline that was standing, so it images the
	// one it displaced. A first dismissal replaces nothing and records an
	// occurrence — which is the difference between the two, spelled by whether
	// there was a prior row rather than by which verb was pressed.
	var auditID ids.UUID
	var err error
	if hadOne {
		before := map[string]any{"permanent": replaced == nil}
		if replaced != nil {
			before = map[string]any{"dismissed_until": replaced.UTC()}
		}
		auditID, err = storekit.Audit(ctx, tx, "update", "contact", contactID.UUID, before, after)
	} else {
		auditID, err = storekit.AuditEvent(ctx, tx, "update", "contact", contactID.UUID, after)
	}
	if err != nil {
		return err
	}
	payload := crmcontracts.PublicEventRelationshipNudgeDecided{
		ContactId: openapi_types.UUID(contactID.UUID),
		Action:    crmcontracts.PublicEventRelationshipNudgeDecidedAction(action),
	}
	if until != nil {
		moment := until.UTC()
		payload.DismissedUntil = &moment
	}
	if permanent {
		payload.Permanent = &permanent
	}
	return storekit.EmitEvent(ctx, tx, auditID, contactID.UUID, payload)
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
		 WHERE d.contact_id = %s.contact_id
		   AND d.reader_id = $%d
		   AND (d.dismissed_until IS NULL OR d.dismissed_until > $%d))`, alias, arg(reader), arg(at)), nil
}
