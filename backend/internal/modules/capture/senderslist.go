// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Every decision this product made about one seat's senders, in one list.
//
// The decisions are already recorded — the ledger holds what the classifier
// concluded, the override table holds what the owner said instead — but they
// live in two tables and neither is readable by the contact they are about. A
// product that decides silently and shows nobody is one an owner has to trust
// rather than check, and the whole posture rests on them being able to check.
//
// One seat's own senders and nobody else's. Whose mail a colleague keeps out is
// itself private: there is no id here that reaches a colleague's list and no
// admin view of one.

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SenderDecision is one sender and what became of them.
type SenderDecision struct {
	Address string
	// Kind is what the classifier concluded — contact, newsletter, personal,
	// advisor and the rest — or empty when it has not answered yet.
	Kind string
	// Status is the ledger's own lifecycle: pending, real, noise, unsure.
	Status string
	// Decision is what the OWNER said instead, empty when they have not.
	Decision string
	// OverruledKind is what their decision overruled, so the page can say
	// "you corrected this" rather than only showing the current answer.
	OverruledKind string
	// RecordExists reports whether a live contact the reader may open holds
	// the address, and ContactID and ContactName name it. All three follow the
	// contact visibility rule, shares included, so none answers for one out of sight.
	RecordExists bool
	ContactID    *ids.UUID
	ContactName  string
	// MessageCount and LastSeenAt describe the stored, unarchived mail this
	// seat imported with the sender as counterparty, in either direction.
	MessageCount int
	LastSeenAt   *time.Time
	// LastSeenPrecision says how to read LastSeenAt, from the message it names.
	LastSeenPrecision string
	// DeletesAt is when this sender's mail is destroyed, set only for a
	// `personal` verdict the owner has not overruled. Zero means nothing is
	// scheduled — every other kind is held rather than destroyed.
	//
	// The EARLIEST message's date, because that is the first thing an owner
	// loses and the deadline the page must name. Later mail from the same
	// sender goes on its own clock, which is why this moves as the oldest is
	// destroyed rather than counting down to one moment.
	DeletesAt time.Time
}

// Overruled reports whether the owner's decision differs from the machine's.
func (d SenderDecision) Overruled() bool { return d.Decision != "" }

// SendersFor lists every sender this seat's mailbox has produced a decision
// about, the owner's own corrections included.
//
// A FULL OUTER JOIN, because the two halves do not cover each other: the ledger
// holds senders nobody has corrected, and the override table holds addresses
// the classifier never reached — including senders whose ledger row a purge
// already deleted, which is exactly the case an owner most needs to still see.
// A list built from either alone would quietly omit one of them.
//
// EACH SIDE IS FILTERED BEFORE THE JOIN, and that placement is the whole
// isolation. Filtering in the ON clause or the WHERE instead lets one side
// widen the other: an override row of the caller's own matches EVERY seat's
// ledger row for that address, the joined row then satisfies the WHERE through
// the caller's own half, and the colleague's `kind` is emitted. A caller could
// write an override for any address they can guess and read back what the
// classifier privately concluded about a colleague's correspondence with it —
// `advisor` among them, which exists to hide that somebody has a lawyer.
func SendersFor(ctx context.Context, db *database.DB, windows PersonalPurgeWindows) ([]SenderDecision, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return nil, apperrors.ErrPermissionDenied
	}
	var out []SenderDecision
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT coalesce(latest.email, o.address) AS address,
			       coalesce(latest.kind, '')         AS kind,
			       coalesce(latest.status, '')       AS status,
			       coalesce(o.decision, '')     AS decision,
			       coalesce(o.overruled_kind, '') AS overruled_kind,
			       -- Only for a personal verdict the owner has not overruled:
			       -- every other kind is held, and an override cancels the sweep,
			       -- so a date on either would name a deletion that never comes.
			       --
			       -- Over the mail the sweep would ACTUALLY take, not over every
			       -- message from the address: PersonalPurgeScope is the same
			       -- clause SelectPersonalPurgeTx applies, so a message under a
			       -- statutory hold or one a colleague also imported does not
			       -- set a date for a deletion that never comes. The sole-import
			       -- test is here rather than in the scope because the sweep
			       -- RELEASES a shared message instead of destroying it — a
			       -- different outcome, not an excluded row.
			       --
			       -- Over EVERY ledger row of the seat for the address, as the sweep
			       -- joins them, not only the newest one the row shows.
			       (SELECT min(`+PersonalPurgeDeadline("$2", "$3")+`)
			          FROM capture_pending_counterparty p
			          JOIN activity a ON a.counterparty_email = p.email
			          JOIN capture_import i ON i.activity_id = a.id AND i.user_id = $1
			         WHERE p.owner_id = $1 AND p.email = coalesce(latest.email, o.address)
			           AND p.kind = 'personal' AND p.status = 'noise' AND p.resolved_at IS NOT NULL
			           AND `+PersonalPurgeScope("$1")+`
			           AND (SELECT count(*) FROM capture_import c
			                 WHERE c.activity_id = a.id) = 1) AS deletes_at
			  -- The newest ledger row per address: an aged-out question and the
			  -- fresh one its sender's next message opened are one sender.
			  FROM (SELECT DISTINCT ON (email) * FROM capture_pending_counterparty
			         WHERE owner_id = $1 ORDER BY email, created_at DESC, id DESC) latest
			  FULL OUTER JOIN (SELECT * FROM capture_sender_override WHERE user_id = $1) o
			    ON o.address = latest.email
			 ORDER BY address`, actor.UserID, windows.ByOwner, windows.ByClassifier)
		if err != nil {
			return fmt.Errorf("capture: listing a seat's sender decisions: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d SenderDecision
			var deletesAt *time.Time
			if err := rows.Scan(&d.Address, &d.Kind, &d.Status,
				&d.Decision, &d.OverruledKind, &deletesAt); err != nil {
				return fmt.Errorf("capture: listing a seat's sender decisions: %w", err)
			}
			if deletesAt != nil {
				d.DeletesAt = *deletesAt
			}
			out = append(out, d)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		addresses := make([]string, len(out))
		for i, d := range out {
			addresses[i] = d.Address
		}
		if err := senderMailTx(ctx, tx, actor.UserID, out, addresses); err != nil {
			return err
		}
		return senderContactsTx(ctx, tx, out, addresses)
	})
	return out, err
}

// senderMailTx counts each sender's mail in this seat's mailbox. Bounded to the
// listed addresses, so the aggregate never walks a whole mailbox.
func senderMailTx(ctx context.Context, tx pgx.Tx, user ids.UUID, out []SenderDecision, addresses []string) error {
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	seat, listed := arg(user), arg(addresses)
	// archived_at alone also excludes restricted mail: activity_restricted_is_archived.
	rows, err := tx.Query(ctx, `
		SELECT a.counterparty_email, count(*), max(a.occurred_at), `+occurredday.LatestPrecisionSQL("a", "")+`
		  FROM activity a
		  JOIN capture_import i ON i.activity_id = a.id AND i.user_id = `+seat+`
		 WHERE a.counterparty_email = ANY(`+listed+`) AND a.kind = 'email' AND a.archived_at IS NULL
		 GROUP BY a.counterparty_email`, args...)
	if err != nil {
		return fmt.Errorf("capture: counting a seat's mail per sender: %w", err)
	}
	defer rows.Close()
	type mail struct {
		count     int
		last      time.Time
		precision string
	}
	seen := map[string]mail{}
	for rows.Next() {
		var address string
		var m mail
		if err := rows.Scan(&address, &m.count, &m.last, &m.precision); err != nil {
			return fmt.Errorf("capture: counting a seat's mail per sender: %w", err)
		}
		seen[address] = m
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("capture: counting a seat's mail per sender: %w", err)
	}
	for i := range out {
		if m, ok := seen[out[i].Address]; ok {
			last := m.last
			out[i].MessageCount, out[i].LastSeenAt, out[i].LastSeenPrecision = m.count, &last, m.precision
		}
	}
	return nil
}

// senderContactsTx finds the contact behind each sender through the rule that
// opens a contact: the read grant, then row scope with its shares. A seat that
// may not see the contact learns neither its id nor that it exists.
func senderContactsTx(ctx context.Context, tx pgx.Tx, out []SenderDecision, addresses []string) error {
	if !auth.ReadGranted(ctx, "contact") {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT pe.email, pe.contact_id, pr.full_name FROM contact_email pe
		  JOIN contact pr ON pr.id = pe.contact_id AND pr.archived_at IS NULL
		 WHERE pe.email = ANY($1) AND pe.archived_at IS NULL
		 ORDER BY pe.email, pe.contact_id`, addresses)
	if err != nil {
		return fmt.Errorf("capture: finding the contacts behind a seat's senders: %w", err)
	}
	defer rows.Close()
	holders := map[string][]ids.UUID{}
	names := map[ids.UUID]string{}
	var candidates []ids.UUID
	for rows.Next() {
		var address string
		var id ids.UUID
		var name string
		if err := rows.Scan(&address, &id, &name); err != nil {
			return fmt.Errorf("capture: finding the contacts behind a seat's senders: %w", err)
		}
		holders[address] = append(holders[address], id)
		names[id] = name
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("capture: finding the contacts behind a seat's senders: %w", err)
	}
	visible, err := auth.VisibleSubset(ctx, tx, "contact", candidates)
	if err != nil {
		return err
	}
	for i := range out {
		for _, id := range holders[out[i].Address] {
			if visible[id] {
				out[i].RecordExists, out[i].ContactID, out[i].ContactName = true, &id, names[id]
				break
			}
		}
	}
	return nil
}
