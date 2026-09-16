// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// One message is one activity, whichever provider hands it over.
//
// An activity's own (source_system, source_id) is where the FIRST arrival filed
// it, and the two doors file differently: captured mail keys on 'email' plus
// the RFC Message-ID, an import keys on its own namespace plus its own record
// id. Those keys never meet, so the same email arriving twice became two rows.
//
// activity_identity is the identity both doors agree on. Its primary key is the
// arbiter: one external identity resolves to exactly one activity, and a second
// arrival claiming it collides there rather than quietly creating a second row.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The identity kinds activity_identity carries, matching its CHECK.
const (
	IdentityKindMail    = "mail"
	IdentityKindMeeting = "meeting"
)

// MeetingIdentityKey is the identity of ONE occurrence of a calendar event.
//
// A recurring series shares a single iCal UID across every occurrence — a
// weekly call is one UID and fifty-two meetings — so the UID names the series
// and the occurrence's own start names the meeting within it. Keying on the UID
// alone would resolve every occurrence to the first one.
func MeetingIdentityKey(icalUID, instance string) string {
	return strings.TrimSpace(icalUID) + "/" + strings.TrimSpace(instance)
}

// ResolveIdentity answers which activity already holds an external identity.
//
// Reports false when the identity is free. A caller that gets an id has found
// the message it was about to store, under whatever name the other door filed
// it: that is the whole mechanism.
func ResolveIdentity(ctx context.Context, tx pgx.Tx, kind, key string) (ids.ActivityID, bool, error) {
	if key == "" {
		return ids.ActivityID{}, false, nil
	}
	var id ids.ActivityID
	err := tx.QueryRow(ctx,
		`SELECT activity_id FROM activity_identity WHERE identity_kind = $1 AND identity_key = $2`,
		kind, key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("activities: resolving a message identity: %w", err)
	}
	return id, true, nil
}

// ClaimIdentity binds an external identity to an activity.
//
// The primary key refuses a second claim on one identity, and that refusal is
// what makes a concurrent claim safe: two arrivals racing on one Message-ID
// both try to insert, one wins, and the loser is told so rather than creating a
// second row. The caller re-resolves and binds to the winner.
//
// Already claimed by THIS activity is not a conflict — a replay re-states what
// it stated before.
func ClaimIdentity(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, kind, key, attestedBy string) error {
	if key == "" {
		return nil
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO activity_identity (identity_kind, identity_key, activity_id, attested_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (identity_kind, identity_key) DO NOTHING`,
		kind, key, activityID, attestedBy)
	if err != nil {
		return fmt.Errorf("activities: claiming a message identity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Somebody holds it. Ours already, or somebody else's — the caller has
		// to look, because the two mean opposite things.
		holder, found, err := ResolveIdentity(ctx, tx, kind, key)
		if err != nil {
			return err
		}
		if !found || holder != activityID {
			return apperrors.ErrConflict
		}
	}
	return nil
}

// IdentitiesOf lists every external identity an activity answers to, so a merge
// can carry them and an erasure can retire them.
func IdentitiesOf(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) ([]string, error) {
	rows, err := tx.Query(ctx,
		`SELECT identity_kind || ':' || identity_key FROM activity_identity
		  WHERE activity_id = $1 ORDER BY identity_kind, identity_key`, activityID)
	if err != nil {
		return nil, fmt.Errorf("activities: listing a message's identities: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("activities: listing a message's identities: %w", err)
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// TransferIdentities moves every identity from one activity to another.
//
// Merging two rows leaves the loser archived with its own key released. Its
// identities have to travel, or they stay pointing at a row nobody can reach —
// a live claim on a dead record, which would send the next arrival of that
// message to the wrong place.
//
// A conflict cannot arise: the survivor and the loser cannot both hold one
// identity, because the primary key is what stopped that.
func TransferIdentities(ctx context.Context, tx pgx.Tx, from, to ids.ActivityID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE activity_identity SET activity_id = $2 WHERE activity_id = $1`,
		from, to); err != nil {
		return fmt.Errorf("activities: carrying a merged message's identities: %w", err)
	}
	return nil
}

// RetireIdentities drops an activity's identities.
//
// Erasure ARCHIVES a row rather than deleting it, so the foreign key's cascade
// never fires. Without this an erased message keeps answering to its
// Message-ID: a later arrival resolves onto a row whose content is gone and
// binds to it, which is both a wrong answer and a way to notice that the
// message was erased.
func RetireIdentities(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM activity_identity WHERE activity_id = $1`, activityID); err != nil {
		return fmt.Errorf("activities: retiring an erased message's identities: %w", err)
	}
	return nil
}

// BindableTo reports whether an arrival may join the activity that already
// holds its identity, and the two cases are not the same question.
//
// A Message-ID is typed by whoever sent the message. Binding two rows together
// means each one's content becomes reachable through the other, so a forged
// identity is a way to reach somebody else's mail.
//
//   - The SAME person wrote both rows → bind. Nobody gains access they did not
//     already have: one human is joining their own record to their own record,
//     which is exactly the import-then-capture case this exists for.
//   - DIFFERENT principals → refuse. The arrival goes on to create its own row.
//     A duplicate is a visible, fixable annoyance; a cross-principal bind on a
//     forged header is not.
//
// Unparseable, absent or non-human on either side is NOT a match. That is the
// one direction this must not fail in: treating "I cannot tell" as "the same
// person" would open the case the rule exists to close.
func BindableTo(ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID) (bool, error) {
	var capturedBy string
	err := tx.QueryRow(ctx,
		`SELECT captured_by FROM activity WHERE id = $1`, incumbent).Scan(&capturedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading who captured a message: %w", err)
	}
	arriving, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	return sameActingHuman(capturedBy, arriving), nil
}

// sameActingHuman reports whether two captured_by stamps name one person.
//
// The stamps are structured: 'human:<uuid>' for somebody writing directly,
// 'connector:<provider>:<uuid>' for a mailbox they connected. One person
// importing their own mail and syncing their own mailbox therefore appears as
// two different stamps carrying one uuid, which is exactly the pair that may
// bind.
//
// Anything with no uuid — 'system', a malformed stamp, an empty one — matches
// nothing, including another copy of itself. Two system writes are not "the
// same person"; they are two writes with no person behind them.
func sameActingHuman(left, right string) bool {
	l, r := actingHumanOf(left), actingHumanOf(right)
	return l != "" && l == r
}

// actingHumanOf pulls the user id out of a captured_by stamp, or answers empty
// when there is no human in it.
func actingHumanOf(capturedBy string) string {
	candidate, ok := trailingIDOf(capturedBy)
	if !ok {
		return ""
	}
	if _, err := ids.Parse(candidate); err != nil {
		// Not a uuid: a stamp shape this does not understand names nobody, and
		// naming nobody must never read as naming the same person.
		return ""
	}
	return candidate
}

// trailingIDOf pulls the id off the end of a captured_by stamp, for the two
// shapes that carry one.
//
// 'human:<uuid>' is somebody writing directly. 'connector:<provider>:<uuid>' is
// a mailbox they connected, where the provider is not part of who they are —
// the same person's Gmail and IMAP stamps must name one person.
func trailingIDOf(capturedBy string) (string, bool) {
	if rest, ok := strings.CutPrefix(capturedBy, "human:"); ok {
		return strings.TrimSpace(rest), true
	}
	rest, ok := strings.CutPrefix(capturedBy, "connector:")
	if !ok {
		return "", false
	}
	idx := strings.LastIndex(rest, ":")
	if idx < 0 {
		// 'connector:gmail' with no seat behind it names a provider, not a
		// person.
		return "", false
	}
	return strings.TrimSpace(rest[idx+1:]), true
}

// EnsureBindableVisible refuses to hand back an activity the caller may not
// read.
//
// Resolving an identity is a READ of the incumbent: the caller learns the
// message is already here and gets its id. Out of scope reads as not-found, so
// the existence of somebody else's message stays hidden and the arrival goes on
// to create its own row.
func EnsureBindableVisible(ctx context.Context, tx pgx.Tx, incumbent ids.ActivityID) error {
	return auth.EnsureActivityVisible(ctx, tx, incumbent.UUID)
}
