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

// StopForCredentialTx records the stop a withdrawal link presses when its
// subject holds no per-purpose consent state to withdraw.
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
// MACHINE LEVEL, not subject level. The press is genuine and unauthenticated
// both: possession of a mailed link is good evidence and not proof of who
// pressed it, and a subject-level row is one no seat may ever lift. An
// operator must be able to correct a mis-scanned link; they must not be able
// to undo a contact's Art. 21 objection. This is the former.
func (s *Store) StopForCredentialTx(ctx context.Context, tx pgx.Tx, ref WithdrawalRef) error {
	if !ref.ContactID.IsZero() {
		return fmt.Errorf("consent: a contact's link withdraws their purposes rather than " +
			"recording an address stop")
	}
	if ref.LeadID.IsZero() && ref.Address == "" {
		return apperrors.ErrNotFound
	}
	// A NAMED-PURPOSE LINK CANNOT BE HONOURED HERE, so it stops nothing rather
	// than stopping too much.
	//
	// communication_suppression has no purpose column: a row binds by KIND,
	// and the objection kind binds every marketing message. So the only stop
	// this table can record for a lead is a broad one, and writing it for a
	// link minted to stop a single subscription would exceed the authority the
	// recipient was handed — they asked to leave one list and would find every
	// marketing message stopped.
	//
	// The narrow stop needs a per-purpose shape for subjects who hold no
	// contact_consent rows, which is a schema question this slice does not
	// answer. Until then a named-purpose lead link is issued and declines to
	// act, which is visible in the audit as nothing happening rather than as
	// the wrong thing happening.
	if ref.Scope == WithdrawalScopeNamedPurpose {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	// SERIALISED FIRST, because NOT EXISTS does not serialise: two concurrent
	// first presses — a mailbox provider retrying while the first is still in
	// flight — both read "no live stop" and both insert, leaving two live rows
	// of one kind. The second lift would then silently re-enable mail the
	// first was still refusing. The lock is the one every writer of this
	// subject's stops takes, so a carry or a lift queues behind this too.
	if err := lockStopKey(ctx, tx, stopSubjectKey(ref)); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO communication_suppression
		    (lead_id, address, kind, source, captured_by, decided_by_level)
		SELECT $1, $2, $3, 'public_link', $4, $5
		 WHERE NOT EXISTS (
		       SELECT 1 FROM communication_suppression live
		        WHERE live.revoked_at IS NULL
		          AND live.kind = $3
		          AND (($1::uuid IS NOT NULL AND live.lead_id = $1)
		            OR ($1::uuid IS NULL AND lower(live.address) = $2)))`,
		zeroAsNull(ref.LeadID.UUID), ref.Address, commsauthz.ReasonObjection,
		by, string(commsauthz.LevelMachine))
	if err != nil {
		return fmt.Errorf("consent: recording the stop this link pressed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already stopped. The press succeeded the first time and this one
		// changes nothing, which is what a replayed one-click POST is.
		return nil
	}
	entity, entityID := entityLead, ref.LeadID.UUID
	if entityID.IsZero() {
		// No record to audit against: the stop is about an address nobody
		// holds. The row itself is the record, and it names its own source.
		return nil
	}
	// AUDITED BUT NOT ANNOUNCED. consent.suppressed declares
	// x-entity-type: contact, so a lead subject would ship an envelope naming
	// contact:<lead uuid> — an id of the wrong kind, scoped by a fan-out gate
	// that was told it could trust the type. stopcarry.go refuses the same
	// event for the same reason. Widening the contract to leads is a question
	// for the slice that asks it.
	if _, err := storekit.AuditEvent(ctx, tx, "update", entity, entityID,
		map[string]any{"stopped": commsauthz.ReasonObjection, "source": "public_link"}); err != nil {
		return err
	}
	return nil
}

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
func (s *Store) StopForCredential(ctx context.Context, token string) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ref, err := resolveWithdrawalTokenTx(ctx, tx, token)
		if err != nil {
			return err
		}
		return s.StopForCredentialTx(ctx, tx, ref)
	})
}

// WithdrawMarketingForCredential stops the MARKETING-CLASS purposes a contact
// holds, for a credential whose scope is all_marketing.
//
// SCOPED BY CLASS, unlike the legacy one-click sweep beside it, which stops
// every purpose in the catalog except the locked transactional one — so a
// press there also withdraws business correspondence, and a contact who
// unsubscribed from a newsletter stops receiving replies to their own
// enquiries. That defect is older than this credential and its fix is a slice
// of its own, because narrowing the LEGACY path changes what an existing link
// in an existing mailbox does.
//
// This credential is new, so nothing depends on it being over-broad, and its
// scope is called all_marketing. Honouring that literally is the only reading
// that matches the words: a link minted to stop marketing stops marketing.
//
// phone_outreach is marketing by another channel and goes with it. The two
// classes left standing are transactional and business_correspondence, which
// are not subscriptions anybody opted into and so not things to unsubscribe
// from.
func (s *Store) WithdrawMarketingForCredential(
	ctx context.Context, contactID ids.ContactID,
) ([]string, error) {
	var keys []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT key FROM consent_purpose
			 WHERE archived_at IS NULL AND class IN ('marketing', 'phone_outreach')
			 ORDER BY key`)
		if err != nil {
			return fmt.Errorf("consent: reading the marketing purposes a link may stop: %w", err)
		}
		keys, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return []string{}, nil
	}
	return s.PublicWithdrawAll(ctx, contactID, keys)
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
