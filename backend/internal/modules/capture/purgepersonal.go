// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which of a seat's personal mail has run out its window and may be destroyed.
//
// `personal` is the one sender kind whose mail the product destroys rather than
// holds: a CRM that keeps a founder's family letters forever, unreadable, has
// still kept them. This file decides WHICH messages that applies to, and it is
// the only thing standing between a verdict and irreversible deletion —
// PurgeActivities takes the ids it is given and performs no check of its own.

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PersonalPurgeWindows is how long a personal verdict waits before its mail is
// destroyed, per authority.
//
// Two windows because two authorities reach the same verdict and they are not
// worth the same. A person who marked a sender personal said so on purpose and
// can say otherwise on the same page; the classifier's answer is a guess nobody
// has yet looked at, and silence is a rep on holiday as readily as it is
// agreement.
type PersonalPurgeWindows struct {
	// ByOwner applies when a person reached the verdict.
	ByOwner string
	// ByClassifier applies when the model did. Longer, deliberately.
	ByClassifier string
}

// DefaultPersonalPurgeWindows is the product's answer: a week for a decision a
// person made, a month for one nobody has confirmed.
func DefaultPersonalPurgeWindows() PersonalPurgeWindows {
	return PersonalPurgeWindows{ByOwner: "7 days", ByClassifier: "30 days"}
}

// PersonalPurgeDeadline is when one message's own window closes, as SQL over an
// `activity a` and a `capture_pending_counterparty p`.
//
// The Senders page shows this date and the sweep acts on it, so both read one
// expression: a page promising a deletion date the sweep disagrees with is worse
// than no date at all. Each caller names where its two window arguments landed,
// because they do not land in the same place — both pass compile-time literals.
func PersonalPurgeDeadline(ownerArg, classifierArg string) string {
	return `greatest(a.created_at, p.resolved_at)
		       + (CASE WHEN p.resolved_by_owner THEN ` + ownerArg + ` ELSE ` + classifierArg + ` END)::interval`
}

// PersonalPurgeScope is which of a seat's mail a personal verdict may destroy,
// setting the window aside — the scope clauses without the deadline.
//
// Exported because the Senders page needs the same answer to name a date: it
// must show when the first message the sweep would ACTUALLY take goes, not when
// the oldest message from that address happens to age out. A message under a
// statutory hold, or one a colleague also imported, is never destroyed, and a
// date computed over those names a deletion that never comes.
//
// Held by: TestOnePlaceDecidesWhenPersonalMailDies (backend/gates/personalpurgewindow_test.go)
func PersonalPurgeScope(seatCol string) string {
	return `-- Mail this seat RECEIVED, captured by a connector. A verdict is
		   -- about a SENDER, and the workspace's own sent mail is its own
		   -- record: destroying the owner's replies because of what was
		   -- concluded about the person they replied to takes away the half of
		   -- the correspondence nobody classified. A hand-logged activity is
		   -- somebody's own work and no capture verdict reaches it.
		   a.kind = 'email' AND a.captured_by LIKE 'connector:%'
		   AND a.direction = 'inbound'
		   AND NOT a.counterparty_outbound_attested
		   -- An obligation the installation owes somebody else outranks a
		   -- verdict about a sender.
		   AND a.restricted_at IS NULL
		   -- Mail already filed against a person is somebody's work, and the
		   -- filing is independent evidence the address is a real correspondent
		   -- whatever the classifier said.
		   AND NOT EXISTS (
		     SELECT 1 FROM activity_link l
		      WHERE l.activity_id = a.id AND l.person_id IS NOT NULL)
		   -- Writing to an address is the T1 signal that they are a real
		   -- counterparty, and it is the recovery path: reply to a wrongly
		   -- judged sender and this sweep lets go.
		   AND NOT EXISTS (
		     SELECT 1 FROM activity c
		      WHERE c.counterparty_email = p.email
		        AND c.direction = 'outbound' AND c.counterparty_outbound_attested)
		   -- The forward bound. counterparty_email comes off an unauthenticated
		   -- From header, so without this one forged message judged personal
		   -- destroys every later mail the REAL owner of that address sends —
		   -- unbounded, and never seen by a human, because a personal verdict
		   -- does not hide the mail and so surfaces nothing to object to.
		   AND a.created_at <= p.resolved_at + ` + quoteInterval(noiseVerdictReach) + `
		   AND NOT EXISTS (
		     SELECT 1 FROM capture_sender_override o
		      WHERE o.user_id = ` + seatCol + ` AND o.address = a.counterparty_email
		        AND o.decision = 'business')`
}

// personalPurgeDue is which personal mail has run out its window, written ONCE
// and read by both the seat census and the per-seat selector.
//
// Held by: TestOnePlaceDecidesWhenPersonalMailDies (backend/gates/personalpurgewindow_test.go)
//
// Two callers ask the same question at different grains — "which seats have work"
// and "which of this seat's messages" — and a second spelling of this predicate
// is how the census would come to disagree with the selector it feeds. Each
// caller supplies the address column and the seat column it joined on; the
// window arguments are $1 (owner) and $2 (classifier) in both.
//
// The seat placeholder is a caller-supplied SQL fragment and never a value off a
// request: both call sites pass a compile-time literal.
func personalPurgeDue(seatCol string) string {
	return `p.status = 'noise' AND p.kind = 'personal'
		   AND p.resolved_at IS NOT NULL
		   AND ` + PersonalPurgeDeadline("$1", "$2") + ` <= now()
		   AND ` + PersonalPurgeScope(seatCol)
}

// SelectPersonalPurgeTx finds the personal mail of one seat whose undo window
// has closed.
//
// The conditions are personalPurgeDue's, and they are the same scope the noise
// sweep applies to the strictly WEAKER effect of hiding — plus a window. That
// is deliberate: this path destroys where the other one hides, so it cannot ask
// for less evidence. `counterparty_email` comes off an unauthenticated From
// header, and personalPurgeDue names what each clause defends.
//
// Three things it adds on top of that scope:
//
//  1. The window is measured PER MESSAGE, from the later of the message's own
//     capture and the verdict — never from the verdict alone. A sender keeps
//     writing after their verdict, and a verdict-keyed window would give mail
//     that arrived a month later no undo window at all, for exactly the
//     messages a wrong verdict is most likely to catch. This is the same
//     reasoning NoiseMailToRedact spells out for the hidden-mail sweep.
//  2. `created_at`, not `occurred_at`. The occurrence time comes off the
//     message's own Date header, which its sender writes: keying the window
//     there lets a forged date destroy a message on arrival.
//  3. No live `business` override for that address. The override is how a
//     person cancels — SenderOverrideStore.Set never touches this ledger, so
//     without the anti-join the overrule writes a row and the purge proceeds
//     anyway.
//
// What it does NOT adopt from the noise scope, and why: `bulk_mail_attested`.
// That is List-Unsubscribe corroboration, which is evidence a message is BULK.
// Personal mail is the opposite kind of thing and carries no such header, so
// requiring it here would refuse every message this sweep exists for. The
// evidence that replaces it is the scope above — inbound, connector-captured,
// unlinked, never written to, inside the verdict's reach — plus a window four
// times longer when no human confirmed the verdict.
//
// Read-only, and shaped like SelectPurgeSubjectTx so a preview and the sweep
// itself get the same answer: the statutory shield is a COLUMN rather than a
// filter, because an owner told their mail is gone must not find it still
// there.
func SelectPersonalPurgeTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, windows PersonalPurgeWindows, floor StatutoryFloor, limit int,
) (PurgeSubject, error) {
	var subject PurgeSubject
	if user == ids.Nil || limit <= 0 {
		return subject, nil
	}
	// Windows first so the shared predicate's $1/$2 mean the same thing in both
	// callers; the seat follows, and the shield appends however many it needs.
	args := []any{windows.ByOwner, windows.ByClassifier, user}
	shielded, args := floor.column(len(args), args)
	// Derived from where the argument actually lands, never typed: the shield
	// contributes a variable number of arguments ahead of it.
	args = append(args, limit)
	limitAt := "$" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, `
		SELECT a.id,
		       (a.restricted_at IS NOT NULL OR (`+shielded+`)) AS withheld,
		       (SELECT count(*) FROM capture_import o WHERE o.activity_id = a.id) AS importers
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $3
		  JOIN capture_pending_counterparty p
		    ON p.email = a.counterparty_email AND p.owner_id = $3
		 WHERE `+personalPurgeDue("$3")+`
		 ORDER BY a.id
		 LIMIT `+limitAt, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: selecting personal mail whose window has closed: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var withheld bool
		var importers int
		if err := rows.Scan(&id, &withheld, &importers); err != nil {
			return subject, fmt.Errorf("capture: selecting personal mail whose window has closed: %w", err)
		}
		switch {
		case withheld:
			subject.Restricted = append(subject.Restricted, id)
		case importers > 1:
			// A colleague imported it too. Their claim is not this seat's to
			// destroy, and a message two mailboxes received is by that fact
			// less likely to be the private correspondence this purge is for.
			subject.SharedImports = append(subject.SharedImports, id)
		default:
			subject.SoleImports = append(subject.SoleImports, id)
		}
	}
	if err := rows.Err(); err != nil {
		return subject, fmt.Errorf("capture: selecting personal mail whose window has closed: %w", err)
	}
	return subject, nil
}

// SeatsWithPersonalMailDueTx lists the seats that have personal mail past its
// window, so a workspace sweep visits only those.
//
// A separate query rather than a loop over every seat: the selector is
// seat-scoped because a purge is, and asking each of a workspace's seats in turn
// would be one query per seat per tick for a condition almost all of them fail.
func SeatsWithPersonalMailDueTx(
	ctx context.Context, tx pgx.Tx, windows PersonalPurgeWindows, limit int,
) ([]ids.UUID, error) {
	if limit <= 0 {
		return nil, nil
	}
	// Derived, not typed, for the same reason as the selector's: personalPurgeDue
	// is one clause away from taking a third window argument, and a typed
	// placeholder would then silently read the wrong value.
	args := []any{windows.ByOwner, windows.ByClassifier, limit}
	limitAt := "$" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT i.user_id
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id
		  JOIN capture_pending_counterparty p
		    ON p.email = a.counterparty_email AND p.owner_id = i.user_id
		 WHERE `+personalPurgeDue("i.user_id")+`
		   -- A restricted message is not work: the selector reports it as
		   -- withheld rather than destroying it, so a seat whose only due mail
		   -- is restricted has nothing for this sweep to do. The per-seat
		   -- selector keeps the row and says so; the census only decides who to
		   -- visit.
		   AND a.restricted_at IS NULL
		 ORDER BY i.user_id
		 LIMIT `+limitAt, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: listing seats with personal mail due: %w", err)
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("capture: listing seats with personal mail due: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: listing seats with personal mail due: %w", err)
	}
	return out, nil
}
