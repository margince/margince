// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a press on a withdrawal link actually does.
//
// Two answers, because two kinds of subject can hold one of these links. A
// CONTACT has per-purpose consent rows, so their press withdraws the marketing
// ones. A LEAD or a bare address has none, so their press records a
// suppression instead — without which their link resolved and then refused,
// which is worse than never issuing it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// sourcePublicLink marks every row a press on a mailed link writes, so a later
// reader can tell an unauthenticated press from a seat's own act.
const sourcePublicLink = "public_link"

// StopForCredentialTx records the stop a withdrawal link presses when its
// subject holds no per-purpose consent state to withdraw, and answers whether
// this press is the one that moved something.
//
// A LEAD HAS NO contact_consent ROWS, and neither does a bare address. The
// per-purpose withdrawal every other press performs therefore has nothing to
// act on, and returning "not found" for those links — which is what this path
// did before — hands a lead a working-looking unsubscribe link that refuses.
// That defeats the mint that issued it: leads getting an opt-out at all is
// half the reason this credential exists.
//
// So the press records a SUPPRESSION instead, against the lead or the bare
// address, which is the shape communication_suppression already carries for
// exactly this case. The send engine reads that table for every message, so a
// stop written here binds the same way a contact's withdrawal does.
//
// THE SCOPE THE LINK CARRIES IS THE SCOPE THE ROW GETS. A named-purpose link
// writes a row narrowed to that purpose (purpose_id set to the purpose the
// link was minted for); an all-marketing link writes the broad row
// (purpose_id NULL) it always did. This used to decline the named-purpose
// case rather than narrow it, on the reasoning that the table had no purpose
// column and a broad row would exceed the authority the recipient was
// handed — true as far as it went, but it meant the link was issued and then
// silently did nothing, which is worse than never issuing it: a lead who
// pressed "stop this list" found every list still arriving. Now that the
// column exists (communication_suppression.purpose_id) and the engine reads
// it (applySuppression in authorizesuppression.go), the press can honour the
// scope it was actually asked for instead of refusing between two wrongs.
//
// MACHINE LEVEL, not subject level. The press is genuine and unauthenticated
// both: possession of a mailed link is good evidence and not proof of who
// pressed it, and a subject-level row is one no seat may ever lift. An
// operator must be able to correct a mis-scanned link; they must not be able
// to undo a contact's Art. 21 objection. This is the former.
//
// FALSE MEANS NOTHING MOVED, which a replayed one-click POST is: the stop was
// already standing. The public handler turns that into the same "nothing
// changed" answer a contact's replayed press gets, so the two subjects stay
// indistinguishable to whoever holds the link.
func (s *Store) StopForCredentialTx(ctx context.Context, tx pgx.Tx, ref WithdrawalRef) (bool, error) {
	if !ref.ContactID.IsZero() {
		return false, fmt.Errorf("consent: a contact's link withdraws their purposes rather than " +
			"recording an address stop")
	}
	if ref.LeadID.IsZero() && ref.Address == "" {
		return false, apperrors.ErrNotFound
	}
	// THE NAMED-PURPOSE ROW NARROWS TO THE LINK'S OWN PURPOSE, and nothing
	// narrower is possible: the link carries one purpose id, so that is the
	// only subscription this press can name. zeroAsNull turns the all-marketing
	// scope's zero-value PurposeID into SQL NULL — the broad row every prior
	// press wrote, unchanged.
	var purposeID *ids.UUID
	if ref.Scope == WithdrawalScopeNamedPurpose {
		purposeID = zeroAsNull(ref.PurposeID)
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	// SERIALISED FIRST, because NOT EXISTS does not serialise: two concurrent
	// first presses — a mailbox provider retrying while the first is still in
	// flight — both read "no live stop" and both insert, leaving two live rows
	// of one kind. The second lift would then silently re-enable mail the
	// first was still refusing. The lock is the one every writer of this
	// subject's stops takes, so a carry or a lift queues behind this too.
	if err := lockStopKey(ctx, tx, stopSubjectKey(ref)); err != nil {
		return false, err
	}
	// THE DEDUP IS ASYMMETRIC, because "already stopped" is.
	//
	// A BROAD live row ABSORBS a narrow press. Somebody who has already left
	// all marketing and then presses one list's link has asked for nothing
	// they do not have: inserting would write a second live objection, audit
	// it, and answer "you are now unsubscribed" to somebody who already was —
	// and leave their Art. 15 export showing two objections for one standing
	// refusal.
	//
	// A NARROW live row does NOT absorb a broad press, nor a press for another
	// purpose. Those ask for strictly more than what stands, so each records.
	//
	// WHAT THIS COSTS, stated because it is a real trade and not a free win:
	// the narrow preference is not written down while the broad row covers it,
	// so lifting the broad stop later would not leave the narrow one behind.
	// That is unreachable today — this door refuses contacts outright, and a
	// lead's stop has no lift path at all (lift.go admits a ContactID only) —
	// so nothing can currently observe the difference. A lift path for leads
	// is the slice that has to decide it, and it should read this comment.
	//
	// `live.purpose_id IS NULL` is the absorbing arm; `= $7` is the exact
	// match. A broad press sends NULL for $7, where `= $7` is never true, so
	// the two arms together leave a broad press matching broad rows only —
	// exactly the behaviour this dedup had before the column existed.
	tag, err := tx.Exec(ctx, `
		INSERT INTO communication_suppression
		    (lead_id, address, kind, source, captured_by, decided_by_level, purpose_id)
		SELECT $1, $2, $3, $6, $4, $5, $7
		 WHERE NOT EXISTS (
		       SELECT 1 FROM communication_suppression live
		        WHERE live.revoked_at IS NULL
		          AND live.kind = $3
		          AND (live.purpose_id IS NULL OR live.purpose_id = $7)
		          AND (($1::uuid IS NOT NULL AND live.lead_id = $1)
		            OR ($1::uuid IS NULL AND lower(live.address) = $2)))`,
		zeroAsNull(ref.LeadID.UUID), ref.Address, commsauthz.ReasonObjection,
		by, string(commsauthz.LevelMachine), sourcePublicLink, purposeID)
	if err != nil {
		return false, fmt.Errorf("consent: recording the stop this link pressed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already stopped. The press succeeded the first time and this one
		// changes nothing, which is what a replayed one-click POST is.
		return false, nil
	}
	entity, entityID := entityLead, ref.LeadID.UUID
	if entityID.IsZero() {
		// No record to audit against: the stop is about an address nobody
		// holds. The row itself is the record, and it names its own source.
		return true, nil
	}
	// AUDITED BUT NOT ANNOUNCED. consent.suppressed declares
	// x-entity-type: contact, so a lead subject would ship an envelope naming
	// contact:<lead uuid> — an id of the wrong kind, scoped by a fan-out gate
	// that was told it could trust the type. stopcarry.go refuses the same
	// event for the same reason. Widening the contract to leads is a question
	// for the slice that asks it.
	//
	// HOW WIDE, alongside what and where from. The kind is the same word for a
	// link that left one list and a link that left all marketing, so an
	// auditor reading this payload alone could not tell the two presses apart
	// — the distinction this press exists to make. Both cases state it rather
	// than the narrow one adding a key, because an absent field reads as one
	// nobody thought to write.
	//
	// THE SCOPE, NOT THE PURPOSE ID. This payload is projected into the
	// History tab a human reads, where a consent_purpose uuid would be the
	// same unreadable thing the subject-access export was just corrected for.
	// Which purpose is on the suppression row itself, for a reader who needs
	// it; what History owes is how far the press reached.
	scope := auditScopeAllMarketing
	if purposeID != nil {
		scope = auditScopeOnePurpose
	}
	if _, err := storekit.AuditEvent(ctx, tx, "update", entity, entityID, map[string]any{
		"stopped": commsauthz.ReasonObjection,
		"source":  sourcePublicLink,
		"scope":   scope,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// The two widths a press's audit row can describe, so the payload and any
// later reader of it agree on the words.
const (
	auditScopeAllMarketing = "all_marketing"
	auditScopeOnePurpose   = "one_purpose"
)

// StopForCredential is the press in its own transaction, for the public
// handler that has none of its own.
//
// IT RE-RESOLVES THE TOKEN INSIDE THAT TRANSACTION rather than trusting a ref
// resolved a moment earlier. The two used to be separate transactions with
// nothing between them: an erasure committing in the gap left the caller
// holding a ref naming a subject and an address that no longer exist, and the
// insert wrote that plaintext address back. An anonymize-in-place leaves the
// lead row standing, so the foreign key does not catch it.
//
// Re-resolving inside the write makes the erasure's own deletion of the
// credential decisive: a token whose row is gone answers not-found here, and
// the press writes nothing.
func (s *Store) StopForCredential(ctx context.Context, token string) (bool, error) {
	var stopped bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		ref, err := resolveWithdrawalTokenTx(ctx, tx, token)
		if err != nil {
			return err
		}
		stopped, err = s.StopForCredentialTx(ctx, tx, ref)
		return err
	}); err != nil {
		return false, err
	}
	return stopped, nil
}

// WithdrawMarketingNamed stops ONE purpose, and only if it is one an
// all-marketing link may stop.
//
// The named-purpose branch of the one-click path used to take the query
// string as given, which is right for a preference token — the mailbox
// provider names the purpose the message was sent under, and second-guessing
// it would refuse a legitimate press. A withdrawal credential is different:
// its scope is part of the credential, so a request naming a purpose outside
// that scope is asking the link to exceed itself.
//
// A purpose outside the marketing classes answers "nothing changed" rather
// than an error. The presser is a mailbox provider acting on a header, not a
// contact who typed something wrong, and a 4xx would turn a press we simply
// decline to widen into a delivery failure they retry.
func (s *Store) WithdrawMarketingNamed(
	ctx context.Context, contactID ids.ContactID, key string,
) ([]string, error) {
	var allowed bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM consent_purpose
			   WHERE key = $1 AND archived_at IS NULL
			     AND class IN ('marketing', 'phone_outreach'))`, key).Scan(&allowed)
	}); err != nil {
		return nil, fmt.Errorf("consent: checking whether this link may stop that purpose: %w", err)
	}
	if !allowed {
		return []string{}, nil
	}
	return s.PublicWithdrawAll(ctx, contactID, []string{key})
}

// stopSubjectKey is what this press locks under: the lead when one holds the
// address, and the address itself when none does — so two presses on an
// address no record names still queue.
func stopSubjectKey(ref WithdrawalRef) string {
	if !ref.LeadID.IsZero() {
		return ref.LeadID.String()
	}
	return ref.Address
}
