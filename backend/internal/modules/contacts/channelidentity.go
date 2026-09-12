// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The channel-identity binding: the one write path for
// contact_channel_identity rows, and the one place the identity race between
// two simultaneous first messages is settled.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// fieldReachability names the channel-reachability image in an audit row and
// in the contact.updated event's changed fields — the same word the contact
// read projects it under (contact_children.go's attachContactReachability), so
// a human reading the trail and a human reading the record see one name.
const fieldReachability = "reachability"

// ResolveOrCreateChannelIdentity binds ci to contactID and returns the contact
// the identity ACTUALLY belongs to once the dust settles — contactID when this
// call created the binding, the incumbent's contact when one already existed.
//
// It is an insert-then-adopt, not a read-then-insert, because the check and
// the act cannot be one statement: two first messages from the same Telegram
// user arrive concurrently, both lanes miss, and both callers would insert.
// Here the loser blocks on Postgres' speculative-insert lock until the winner
// commits, then reads the winner out and adopts it. The database is the
// arbiter, so there is no window to lose.
//
// A caller that had to CREATE a contact before it could offer one must treat a
// returned id different from contactID as having lost the race: its own contact
// row is speculative and must not survive the transaction, or the human ends
// up on two records with the conversation on one of them.
//
// The row carries no audit entry of its own, by the same rule as contact_email
// and contact_phone: it is a satellite of the contact write that encloses it,
// and that write's audit row is the one an auditor reads.
func ResolveOrCreateChannelIdentity(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, ci connector.ChannelIdentity) (ids.ContactID, error) {
	if ci.Provider == "" || ci.ChannelUserID == "" {
		return ids.ContactID{}, errors.New("contacts: a channel identity needs both a provider and a channel user id")
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.ContactID{}, err
	}
	// source names the channel the binding came from; the provider IS that
	// channel, and unlike a mail message there is no per-record id worth
	// stamping — the binding outlives every message that refreshed it.
	tag, err := tx.Exec(ctx, `
		INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $2, $5)
		ON CONFLICT (provider, channel_user_id) WHERE archived_at IS NULL
		DO NOTHING`,
		contactID, ci.Provider, ci.ChannelUserID, ci.Username, by)
	if err != nil {
		return ids.ContactID{}, fmt.Errorf("contacts: binding channel identity: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return contactID, nil
	}

	// Zero rows means a live binding already exists — either it predates this
	// call or the concurrent winner has now committed. A fresh statement takes
	// a fresh snapshot under READ COMMITTED, so this read sees it.
	var winner ids.ContactID
	err = tx.QueryRow(ctx, `
		SELECT contact_id FROM contact_channel_identity
		WHERE provider = $1 AND channel_user_id = $2 AND archived_at IS NULL`,
		ci.Provider, ci.ChannelUserID).Scan(&winner)
	if err != nil {
		return ids.ContactID{}, fmt.Errorf("contacts: reading the channel identity that won the bind: %w", err)
	}
	return winner, nil
}

// SetChannelIdentityBlocked applies a my_chat_member reachability change
// (design §4.2 D9): a block sets blocked_at, an unblock clears it. It NEVER
// touches archived_at — archiving is the same trap this file's package
// comment warns against for the bind path: the dedupe lane and the unique
// index both read archived_at IS NULL, so archiving on block would fork a
// returning customer into a second Contact the moment their next message
// misses the lane.
//
// updateID is Telegram's per-bot sequence number for the update that reports
// the change, and the row remembers the last one it applied. Telegram numbers
// its updates but the ingest queue runs several workers, so a block and the
// unblock answering it can arrive at this write in either order; without that
// memory the loser is applied last and a reachable customer is left suppressed
// for good, since nothing else ever writes blocked_at.
//
// The watermark advances on every update the row accepts, INCLUDING one that
// leaves the state as it found it — a newer same-state update that recorded
// nothing would leave the row still willing to accept an older update of the
// opposite state.
//
// botID scopes the comparison because update_id counts per bot: replacing the
// workspace's bot restarts the sequence low, and ids from two different bots
// do not order each other at all. IS DISTINCT FROM starts the new bot's
// sequence from scratch; comparing across bots instead would read every update
// from the replacement as stale and wedge the identity's reachability
// permanently.
//
// The write stays idempotent under Telegram's redelivery, which is the same
// update and so carries the same update_id: a repeat touches zero rows — it
// does not move blocked_at's timestamp forward, re-fire the
// updated_at/version trigger, or leave a second audit row for a state that did
// not change. Only a genuine flip is audited, which is why the pre-update
// image is read in the same statement rather than trusted from the arguments.
//
// A my_chat_member naming an identity nobody has bound yet (blocked before
// ever messaging the bot) matches no row. That is not a fault: there is no
// reachability state yet to correct.
func (s *Store) SetChannelIdentityBlocked(ctx context.Context, tx pgx.Tx, ci connector.ChannelIdentity, blocked bool, botID string, updateID int64) error {
	if ci.Provider == "" || ci.ChannelUserID == "" {
		return errors.New("contacts: a channel identity needs both a provider and a channel user id")
	}
	// Telegram numbers its updates from 1 up and always names the bot that
	// received them, so neither of these comes off the wire: a zero id would be
	// stored as a watermark no later update could exceed, and an unnamed bot
	// would order two unrelated sequences against each other. Both leave the
	// identity's reachability frozen, which is the failure this write exists to
	// prevent, so it refuses rather than applies.
	if botID == "" || updateID <= 0 {
		return fmt.Errorf(
			"contacts: a reachability change needs the bot that received it and that bot's update id, got bot %q and update %d",
			botID, updateID)
	}
	// At most one row can come back: the partial unique index admits one live
	// binding per (workspace, provider, channel_user_id).
	var contactID ids.ContactID
	var wasBlockedAt *time.Time
	err := tx.QueryRow(ctx, `
		WITH bound AS (
			SELECT id, contact_id, blocked_at FROM contact_channel_identity
			 WHERE provider = $1 AND channel_user_id = $2 AND archived_at IS NULL
		)
		UPDATE contact_channel_identity pci
		   SET membership_bot_id    = $4,
		       membership_update_id = $5,
		       blocked_at = CASE WHEN $3 THEN coalesce(pci.blocked_at, now()) ELSE NULL END
		  FROM bound
		 WHERE pci.id = bound.id
		   AND (pci.membership_bot_id IS DISTINCT FROM $4
		        OR $5 > coalesce(pci.membership_update_id, 0))
		RETURNING bound.contact_id, bound.blocked_at`,
		ci.Provider, ci.ChannelUserID, blocked, botID, updateID).Scan(&contactID, &wasBlockedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: setting channel identity blocked=%t: %w", blocked, err)
	}
	if (wasBlockedAt != nil) == blocked {
		return nil
	}
	return auditChannelIdentityChange(ctx, tx, contactID,
		reachabilityImage(ci.Provider, blocked), reachabilityImage(ci.Provider, !blocked))
}

// reachabilityImage is the audit/event field image for a reachability flip.
// The provider travels with it because a contact can hold identities on
// several channels, and "reachable: false" alone would not say on which.
func reachabilityImage(provider string, reachable bool) map[string]any {
	return map[string]any{fieldReachability: map[string]any{
		"provider": provider, "reachable": reachable,
	}}
}

// auditChannelIdentityChange puts a channel-identity mutation on the CONTACT's
// write trail: domain row + audit row + outbox event in the caller's one
// transaction.
//
// The contact is the right subject, not the satellite. Both mutations this
// serves change what the RECORD says — reachability is projected onto the
// contact read (contact_children.go), and it decides whether a rep is offered
// the reply box at all — so an auditor asking "why can this contact no longer
// be messaged, and since when" must find the answer on the contact's history,
// exactly as a contact_profile_field write is audited as a contact update
// (enrichsignature.go).
//
// The bind path needs none of this: it is enclosed by the contact create whose
// audit row already covers it. These two writes have no enclosing contact
// mutation — they reach a Contact who already exists.
//
// Every genuine change gets a row, and there is no per-identity budget on top
// of that. The counterpart decides how often they flip, so the question is what
// bounds the trail, and the answer is where the bound belongs rather than here:
//
//   - Repetition costs nothing. Both callers guard on the CURRENT stored state,
//     so a redelivered my_chat_member and a message repeating the handle we
//     already hold reach no row and write nothing — which covers everything
//     Telegram generates on its own.
//   - What is left is one row per state a human actually changed, and a human
//     who can change state can already send messages, each of which costs an
//     activity, an audit row and an event of its own. A budget here would cap a
//     constant factor on a path that is unbounded by design, and the real bound
//     on both is the same one: the workspace disconnects the bot.
//   - A capped trail would be worse than a long one. Reachability decides
//     whether a rep is offered the reply box at all, so a flip recorded nowhere
//     leaves an auditor asking "since when can we not message this contact" with
//     a record that changed and a history that does not say so — and a trail
//     silently truncated by a budget reads exactly like a complete one.
func auditChannelIdentityChange(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, before, after map[string]any) error {
	// Binding an account to a contact who had none replaces nothing: the
	// question an auditor brings is which account reaches this human, and
	// before the bind the answer was that none did. A rebind moved a binding
	// and says which one it moved.
	var auditID ids.UUID
	var err error
	if before == nil {
		auditID, err = storekit.AuditEvent(ctx, tx, actionUpdate, entityContact, contactID.UUID, after)
	} else {
		auditID, err = storekit.Audit(ctx, tx, actionUpdate, entityContact, contactID.UUID, before, after)
	}
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, contactID.UUID,
		crmcontracts.PublicEventContactUpdated{ChangedFields: after})
}

// ReachableChannelIdentities returns every identity contactID can currently be
// REACHED at on provider — a live row with blocked_at IS NULL (design §6.6).
//
// It returns a list rather than one identity because a contact may hold more than
// one account on the same channel: the unique key is
// (workspace, provider, channel_user_id), which binds an account to one contact
// and not a contact to one account. Handing the caller the first row would reply
// to whichever account the planner returned, so the choice is theirs to refuse.
//
// An EMPTY list is an answer, not a fault: a contact who never messaged the
// workspace's bot and one who blocked it are both simply unreachable, and the
// caller owes the rep that sentence rather than an error. Only a failure to ASK
// is an error.
//
// The username travels because a caller may show it; nothing routes on it, since
// a handle can be released and re-claimed while the account id cannot.
func ReachableChannelIdentities(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, provider string) ([]connector.ChannelIdentity, error) {
	rows, err := tx.Query(ctx, `
		SELECT channel_user_id, coalesce(username, '')
		  FROM contact_channel_identity
		 WHERE contact_id = $1 AND provider = $2
		   AND archived_at IS NULL AND blocked_at IS NULL
		 ORDER BY created_at`, contactID, provider)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading reachable channel identities: %w", err)
	}
	defer rows.Close()
	var out []connector.ChannelIdentity
	for rows.Next() {
		identity := connector.ChannelIdentity{Provider: provider}
		if err := rows.Scan(&identity.ChannelUserID, &identity.Username); err != nil {
			return nil, fmt.Errorf("contacts: reading reachable channel identities: %w", err)
		}
		out = append(out, identity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading reachable channel identities: %w", err)
	}
	return out, nil
}
