// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What THIS mailbox asks of one message, decided inside the capture
// transaction.
//
// The order is the whole design. Every rule that can only TIGHTEN runs before
// the one rule that can loosen, so there is no ordering of them under which a
// message is briefly readable and then held: the answer this returns is the
// answer the row is born with, and a message that should be held was never
// anything else.
//
// This decides for one seat. What the ROW ends at is the strictest across every
// seat that imported it, which activities.RecomputeAudienceTx derives from the
// capture_import rows this writes.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// birthDecision is what one mailbox concluded about one message at capture: the
// posture it was under, and the verdict state that follows from it.
//
// posture and verdictStatus are written onto this seat's capture_import row, so
// the derivation reads them rather than re-deriving anything. reason is carried
// separately for the two holds that belong to no mailbox — the workspace floor
// and a counterparty hold — because those are facts about the workspace or the
// correspondent rather than about the mailbox's own posture.
type birthDecision struct {
	posture       string
	verdictStatus string
	reason        string
}

// decideBirthTx answers what this mailbox asks of this message.
//
// Five steps, strictest first:
//
//  1. the workspace mail-sharing floor — off holds everything, whatever any
//     mailbox asks;
//  2. a counterparty hold this seat placed — their lawyer's domain, held
//     whatever the thread turns out to be about;
//  3. an explicit marker on this very message — a subject saying
//     [Vertraulich] is the sender telling us, and no classifier is needed to
//     read it;
//  4. a prior verdict on this thread, for this seat — but only when the
//     message's sender is an address that verdict actually SAW;
//  5. this mailbox's own posture.
//
// Steps 1 to 4 can only hold. Step 5 is the only one that can open a message,
// and it runs last, so nothing above it can be overturned by it.
func decideBirthTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (birthDecision, error) {
	// Non-mail kinds keep the workspace default: a meeting or a channel message
	// is not correspondence a mailbox posture was ever asked about.
	if fields.Kind != "email" {
		return birthDecision{}, nil
	}

	sharing, err := settings.ApplyTx(ctx, tx, MailSharing)
	if err != nil {
		return birthDecision{}, fmt.Errorf("capture: reading the mail-sharing posture: %w", err)
	}
	if !sharing {
		return birthDecision{posture: PostureHeld, reason: audienceReasonWorkspaceFloor}, nil
	}

	held, err := heldCounterpartyTx(ctx, tx, rec)
	if err != nil {
		return birthDecision{}, err
	}
	if held {
		return birthDecision{posture: PostureHeld, reason: audienceReasonCounterparty}, nil
	}

	if explicitlyConfidential(fields.Subject) {
		// The marker also re-opens a settled thread: a reply saying
		// [Vertraulich] on a conversation a classifier cleared last week is new
		// information about that conversation, and leaving the cleared verdict
		// standing would let the NEXT message on it inherit an answer this
		// message just contradicted.
		if err := reopenClearedThreadTx(ctx, tx, rec.ThreadKey); err != nil {
			return birthDecision{}, err
		}
		return birthDecision{posture: PostureHeld, reason: audienceReasonConfidentialMarker}, nil
	}

	inherited, err := inheritedVerdictTx(ctx, tx, rec)
	if err != nil {
		return birthDecision{}, err
	}
	if inherited != "" {
		return birthDecision{verdictStatus: inherited}, nil
	}

	posture, err := mailboxPostureTx(ctx, tx)
	if err != nil {
		return birthDecision{}, err
	}
	return birthDecision{posture: posture}, nil
}

// mailboxPostureTx reads the posture of the connection that DELIVERED this
// message.
//
// The acting principal names both halves: its UserID is the granting seat and
// its ID is `connector:<provider>`, which together identify one row. A seat may
// hold several mailboxes — a personal Gmail and a shared team Outlook — and
// asking for "their newest connection" would answer with whichever was
// connected last rather than the one this message came through, so a message
// arriving in a held mailbox could be born open because the seat later
// connected a shared one.
//
// Anything the query cannot pin down answers `held`. The seat cannot be asked
// what they want mid-sync, and a message whose provenance the product cannot
// establish is the last one to publish on a guess.
func mailboxPostureTx(ctx context.Context, tx pgx.Tx) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return PostureHeld, nil
	}
	provider := strings.TrimPrefix(actor.ID, "connector:")
	if provider == "" || provider == actor.ID {
		// Not a connector principal, so there is no connection to read a
		// posture from.
		return PostureHeld, nil
	}
	var posture string
	err := tx.QueryRow(ctx, `
		SELECT mail_posture FROM capture_connection
		 WHERE user_id = $1 AND provider = $2 AND archived_at IS NULL`,
		actor.UserID, provider).Scan(&posture)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PostureHeld, nil
		}
		return "", fmt.Errorf("capture: reading the mailbox posture: %w", err)
	}
	return posture, nil
}

// heldCounterpartyTx answers whether this seat holds any party to this message.
//
// The SQL shape excludedTx uses, and for the same reason: a domain rule covers
// its subdomains, so holding studiolegal.de holds mail.studiolegal.de too.
// Scoped to THIS seat — a hold is one person's decision about their own
// correspondence, and a workspace-wide one would let anyone hold a colleague's
// customer out of the shared CRM.
func heldCounterpartyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	user := actorUserID(ctx)
	if user == ids.Nil || len(rec.Addresses) == 0 {
		return false, nil
	}
	addresses, domains := foldedAddressesAndDomains(rec.Addresses)
	var found bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM capture_counterparty_hold h
			 WHERE h.user_id = $3
			   AND ((h.kind = 'address' AND h.value = ANY($1::text[]))
			     OR (h.kind = 'domain' AND EXISTS (
			           SELECT 1 FROM unnest($2::text[]) d
			            WHERE d = h.value OR d LIKE '%.' || h.value))))`,
		addresses, domains, user).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("capture: reading this seat's counterparty holds: %w", err)
	}
	return found, nil
}

// foldedAddressesAndDomains splits one message's parties into the two lists a
// hold or exclusion lookup matches on, folded the way this module stores every
// address so neither side has to case-fold at query time.
func foldedAddressesAndDomains(raw []string) (addresses, domains []string) {
	addresses = make([]string, 0, len(raw))
	domains = make([]string, 0, len(raw))
	for _, a := range raw {
		addresses = append(addresses, strings.ToLower(strings.TrimSpace(a)))
		if d := domainOfAddress(a); d != "" {
			domains = append(domains, d)
		}
	}
	return addresses, domains
}

// bornAudience renders the decision as the audience column the insert writes.
//
// The row is born at the answer this seat asked for, and the recompute that
// runs later in the same transaction derives the row's final audience across
// every importing seat. Writing it here rather than leaving it to the recompute
// is what closes the window: an insert that landed `workspace` and was narrowed
// a few statements later is still a row that was `workspace`, and a concurrent
// reader in another transaction can see exactly that.
func (d birthDecision) bornAudience() (audience, reason string) {
	switch d.verdictStatus {
	case VerdictCleared, VerdictSharedByOwner:
		return audienceWorkspace, ""
	case VerdictHeld, VerdictUnsure, VerdictHeldByOwner, VerdictPending:
		return audienceParticipants, audienceReasonPendingVerdict
	}
	switch d.posture {
	case PostureHeld, PostureClassified:
		if d.reason != "" {
			return audienceParticipants, d.reason
		}
		return audienceParticipants, audienceReasonPosture
	}
	return audienceWorkspace, ""
}
