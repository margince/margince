// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The strength dates a contact row stores, so the company page and the list
// reads fold no history. Statement triggers keep them at what
// contact_strength_dates gives at now(), in the writer's transaction, and
// recompute them when a change can lower one. They answer for the present only.

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// strengthPresentSlack is how far the caller's now may trail the database's
// and still be the present the stored dates were folded at.
const strengthPresentSlack = 5 * time.Minute

// strengthClockSkew is how far the caller's now may run ahead of the
// database's: the skew between the two hosts' clocks. A request takes its now
// before its transaction begins and can still show it. A caller further ahead
// folds the history, because its now may admit what the dates left out.
const strengthClockSkew = time.Second

// storedPrecisionSQL reads a stored day flag back as the precision it records.
func storedPrecisionSQL(day string) string {
	return `CASE WHEN ` + day + ` THEN 'day' WHEN NOT ` + day + ` THEN 'instant' END`
}

// storedStrengths folds the contacts from their stored dates in one statement.
// It also runs the window read for each contact last met inside the window.
// A contact the dates cannot answer for is folded live. That is one the
// backfill has not reached, or any contact when now is not the database's present.
func storedStrengths(ctx context.Context, tx pgx.Tx, contacts []ids.ContactID, now time.Time) (map[ids.ContactID]*RelationshipStrength, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	contactsPos := arg(contacts)
	visible, err := contactScopePredicate(ctx, arg)
	if err != nil {
		return nil, err
	}
	from, at := fmt.Sprintf("$%d", arg(now.AddDate(0, 0, -relStrengthWindowDays))), fmt.Sprintf("$%d::timestamptz", arg(now))
	slack := fmt.Sprintf("make_interval(secs => $%d)", arg(strengthPresentSlack.Seconds()))
	skew := fmt.Sprintf("make_interval(secs => $%d)", arg(strengthClockSkew.Seconds()))
	usable := `(p.strength_folded AND ` + at + ` BETWEEN now() - ` + slack + ` AND now() + ` + skew + `
		          AND (p.strength_last_interaction_at IS NULL OR p.strength_last_interaction_at <= ` + at + `))`
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT p.id, `+usable+`,
		       p.strength_last_interaction_at, p.strength_last_engaged_at,
		       p.strength_last_inbound_at, `+storedPrecisionSQL("p.strength_last_inbound_day")+`,
		       p.strength_last_outbound_at, `+storedPrecisionSQL("p.strength_last_outbound_day")+`,
		       c.*
		  FROM contact p
		 CROSS JOIN LATERAL (`+strengthWindowSQL("p.id", "p.strength_last_engaged_at", from, at,
		usable+" AND p.strength_last_interaction_at >= "+from, "")+`) c
		 WHERE p.id = ANY($%d) AND p.archived_at IS NULL AND (`+visible+`)`, contactsPos), args...)
	if err != nil {
		return nil, err
	}
	byContact := map[ids.ContactID]*RelationshipStrength{}
	var unfolded []ids.ContactID
	for rows.Next() {
		var contactID ids.ContactID
		var answers bool
		var dates recencyDates
		rs := &RelationshipStrength{}
		if err := rows.Scan(append(append([]any{&contactID, &answers}, dates.targets(rs)...), rs.windowCounts()...)...); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case !answers:
			unfolded = append(unfolded, contactID)
		case rs.LastInteraction != nil:
			dates.apply(rs)
			byContact[contactID] = rs
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(unfolded) == 0 {
		return byContact, nil
	}
	live, err := liveStrengths(ctx, tx, unfolded, foldScope{admit: now})
	maps.Copy(byContact, live)
	return byContact, err
}

// RefreshStrengthDatesTx folds the stored dates of each contact again from its
// history at now(). The clock refold calls it once an activity's time has come,
// because no row changes then, and the backfill calls it for each live contact.
func RefreshStrengthDatesTx(ctx context.Context, tx pgx.Tx, contacts []ids.UUID) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if len(contacts) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT refresh_contact_strength_dates($1::uuid[])`, contacts); err != nil {
		return fmt.Errorf("contacts: refolding stored strength dates: %w", err)
	}
	return nil
}

// UnfoldStrengthDatesTx marks every live contact's stored dates unfolded, so
// readers fold the history until the clock refold has folded them again. A
// move of the installation zone calls it. Whether a day entry has happened
// depends on the zone's today, so dates folded in the old zone may be wrong.
func UnfoldStrengthDatesTx(ctx context.Context, tx pgx.Tx) error {
	if err := auth.Require(ctx, "installation_settings", principal.ActionUpdate); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT unfold_contact_strength_dates()`); err != nil {
		return fmt.Errorf("contacts: marking stored strength dates unfolded: %w", err)
	}
	return nil
}

// UnfoldedContactsTx answers up to limit live contacts whose stored dates
// are not folded, in id order.
func UnfoldedContactsTx(ctx context.Context, tx pgx.Tx, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id FROM contact WHERE NOT strength_folded AND archived_at IS NULL ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading unfolded strength dates: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// DirectionlessSendersPageTx reads the next limit contacts after `after`, in
// id order. It answers every id it inspected, for the cursor, and the live ones
// that wrote a message stored with no direction, whose dates that rule moves.
func DirectionlessSendersPageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (inspected, selected []ids.UUID, err error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, nil, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.archived_at IS NULL AND EXISTS (
		       SELECT 1 FROM activity_link l JOIN activity a ON a.id = l.activity_id
		        WHERE l.contact_id = p.id AND a.direction IS NULL AND `+relstrength.TheyWroteSQL("p.id", "a")+`)
		  FROM contact p WHERE p.id > $1 ORDER BY p.id LIMIT $2`, after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("contacts: reading senders of direction-less mail: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var wrote bool
		if err := rows.Scan(&id, &wrote); err != nil {
			return nil, nil, fmt.Errorf("contacts: reading senders of direction-less mail: %w", err)
		}
		inspected = append(inspected, id)
		if wrote {
			selected = append(selected, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("contacts: reading senders of direction-less mail: %w", err)
	}
	return inspected, selected, nil
}
