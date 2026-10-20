// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What an inbound mail's envelope says about a stranger's mail, judged once
// at capture. The waiting queue reads the facts to keep a stranger's Bcc or
// foreign-link mail out of its top band.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/captureorigin"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// envelopeFacts are capture_import.bcc_only for this seat, and the activity's
// foreign_link and sender_was_stranger. Nil is NULL: not judged, which the
// queue reads as no evidence.
type envelopeFacts struct {
	bccOnly, foreignLink, senderWasStranger *bool
}

// judgeEnvelopeTx judges one inbound mail. Anything else, and list mail,
// gets no facts at all.
func judgeEnvelopeTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (envelopeFacts, error) {
	envelope, sender := fields.Envelope, rec.Counterparty.Email
	if envelope == nil || fields.Kind != kindEmail || fields.Direction != connector.DirectionInbound || sender == "" {
		return envelopeFacts{}, nil
	}
	stranger, err := senderWasStrangerTx(ctx, tx, sender, receivedAt(rec, fields))
	if err != nil {
		return envelopeFacts{}, err
	}
	foreign, err := foreignLinkTx(ctx, tx, sender, envelope)
	if err != nil {
		return envelopeFacts{}, err
	}
	facts := envelopeFacts{foreignLink: &foreign, senderWasStranger: &stranger}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return envelopeFacts{}, err
	}
	// With no seat behind the capture, no address of ours is known. The Bcc
	// question then stays unanswered rather than answered yes.
	if !self.Empty() {
		bcc := !addressedOpenly(envelope.OpenRecipients, rec.DeliveredTo, self)
		facts.bccOnly = &bcc
	}
	return facts, nil
}

// receivedAt is when the mail reached the mailbox: the provider's own time,
// else the mail's Date header. captureActivity has already set a missing Date
// to the capture time, so this is never zero.
func receivedAt(rec connector.NormalizedRecord, fields ActivityFields) time.Time {
	if !rec.ProviderReceivedAt.IsZero() && datasource.InstantInRange(rec.ProviderReceivedAt) {
		return rec.ProviderReceivedAt
	}
	return fields.OccurredAt
}

// addressedOpenly reports whether To or Cc names this seat's mailbox: one of
// the seat's own addresses, or the trusted address the mail was delivered to.
// A seat's domain claim does not count: it names every colleague too, and a
// colleague in To or Cc leaves this seat a Bcc copy.
func addressedOpenly(recipients []string, deliveredTo string, self SelfSet) bool {
	delivered := foldAddress(deliveredTo)
	for _, recipient := range recipients {
		if self.namesSeat(recipient) || (delivered != "" && foldAddress(recipient) == delivered) {
			return true
		}
	}
	return false
}

// namesSeat is Covers without the domain arm: an address of the seat's own,
// proven or identity-only.
func (s SelfSet) namesSeat(address string) bool {
	folded := foldAddress(address)
	_, proven := s.addresses[folded]
	_, identity := s.identityOnly[folded]
	return folded != "" && (proven || identity)
}

// senderWasStrangerTx reports whether, strictly before `before`, the sender
// was no live contact and the workspace had never written to them. A later
// contact or reply must not make an earlier mail look known.
//
// A contact capture minted from mail does not count: a stranger's first mail
// would otherwise make their second one known. A human's, an import's or an
// agent's contact does.
func senderWasStrangerTx(ctx context.Context, tx pgx.Tx, sender string, before time.Time) (bool, error) {
	spellings := addressSpellings(sender)
	var contact bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM contact_email pe JOIN contact p ON p.id = pe.contact_id
		   WHERE pe.email = ANY($1::text[]) AND pe.archived_at IS NULL
		     AND p.archived_at IS NULL AND p.merged_into_id IS NULL AND p.created_at < $2
		     AND NOT (`+captureorigin.ContactSQL("$3", "$4", "$5")+`))`,
		spellings, before, captureorigin.VerdictActor, captureorigin.AcceptActor, captureorigin.VCardSource).Scan(&contact)
	if err != nil {
		return false, fmt.Errorf("capture: asking whether a sender is a contact: %w", err)
	}
	if contact {
		return false, nil
	}
	corresponded, err := correspondedBeforeTx(ctx, tx, spellings, before)
	return !corresponded, err
}

// addressSpellings is an address as stored columns may hold it: lowercased,
// and with its domain in IDNA form too. A unicode domain may be stored either way.
func addressSpellings(address string) []string {
	spellings := []string{correspondence.Fold(address)}
	if canonical := foldAddress(address); !slices.Contains(spellings, canonical) {
		spellings = append(spellings, canonical)
	}
	return spellings
}
