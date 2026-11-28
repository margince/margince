// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Mail a seat forwarded to their own address. Between their own addresses
// alone it would be dropped as internal, and the customer mail it carries
// would be lost. Read here, it is filed under the original's sender, marked as
// a forward nobody verified, and stays the seat's own message.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The skip reason for mail between the seat's own addresses that carries no
// forward a customer can be read from.
const (
	reasonOwnAddressesOnly = string(pipelinetrace.ReasonOwnAddressesOnly)
	skipOwnAddressesOnly   = "every party is one of the mailbox owner's own addresses"
)

// readForwardToSelfTx re-reads mail the seat forwarded to their own address as
// mail about the original's sender.
//
// The original's From is text the forwarder could edit. It decides only whom
// the message is filed under, which the seat could have done by hand. It never
// becomes the message's author or its counterparty: the seat stays both.
// Five things must hold:
//   - the provider filed the mail as sent, so the seat sent it and no stranger
//     spoofed their From. A copy only in the Inbox stays internal;
//   - every party is one of the seat's own addresses, and none is unreadable;
//   - it went to the connected mailbox itself. A declared private alias is where
//     private mail goes, and it stays private;
//   - the original's sender is nobody in this workspace, nor on its domains;
//   - it is the seat's own mailbox: a shared mailbox's run has no seat to file for.
func (s *Sink) readForwardToSelfTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (connector.NormalizedRecord, error) {
	fields, ok := rec.Fields.(ActivityFields)
	original := rec.Forwarded
	if !ok || original == nil || original.From == "" || fields.Kind != kindEmail || fields.HasCalendarPart ||
		!rec.Counterparty.FiledAsSent() {
		return rec, nil
	}
	if _, shared := sharedMailboxOf(ctx); shared {
		return rec, nil
	}
	eligible, err := forwardEligibleTx(ctx, tx, rec, fields, original)
	if err != nil || !eligible {
		return rec, err
	}
	if fields.forwardThread, err = originalThreadTx(ctx, tx, rec, original); err != nil {
		return rec, err
	}
	fields.forwardedFrom = original
	// Only text quoted from the original is not the seat's own words. A forward
	// that attaches the original keeps its note, and mailmap judged that body.
	if original.Inline && fields.BodyCompleteness.stored() == BodyComplete {
		fields.BodyCompleteness = BodyForwarded
	}
	rec.Fields = fields
	// The original's sender is a party now, so the internal gate keeps the
	// message and an exclusion of that sender still keeps it out.
	rec.Addresses = append(slices.Clone(rec.Addresses), original.From)
	// Its own conversation. A forward is no answer to the original, and joining
	// that thread would read the seat as having replied to the customer.
	rec.ThreadKey, rec.ReplyTo = rec.NaturalKey.SourceID, nil
	return rec, nil
}

// forwardEligibleTx is the four checks of readForwardToSelfTx that read the
// seat's addresses and the workspace.
func forwardEligibleTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields, original *connector.ForwardedOriginal,
) (bool, error) {
	own, err := ownAddressesOnlyTx(ctx, tx, rec)
	if err != nil || !own {
		return false, err
	}
	toMailbox, err := sentToConnectedMailboxTx(ctx, tx, fields)
	if err != nil || !toMailbox {
		return false, err
	}
	us, err := workspacePartiesTx(ctx, tx, original.From)
	if err != nil || us.Covers(original.From) {
		return false, err
	}
	domains, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return !domains.AllInternal([]string{original.From}), nil
}

// sentToConnectedMailboxTx reports mail whose every open recipient is the
// address of the mailbox that captured it (seatMailboxTx). Another of the
// seat's mailboxes is not this one, and a run that names none answers no.
func sentToConnectedMailboxTx(ctx context.Context, tx pgx.Tx, fields ActivityFields) (bool, error) {
	if fields.Envelope == nil || len(fields.Envelope.OpenRecipients) == 0 {
		return false, nil
	}
	actor, ok := principal.Actor(ctx)
	provider := strings.TrimPrefix(actor.ID, "connector:")
	if !ok || actor.UserID == ids.Nil || provider == actor.ID {
		return false, nil
	}
	connection, found, err := seatMailboxTx(ctx, tx, actor.UserID, provider)
	if err != nil || !found {
		return false, err
	}
	match := memberConnection(actor.UserID, connection)
	var label string
	err = tx.QueryRow(ctx, `SELECT coalesce(account_label, '') FROM capture_connection WHERE `+match.where,
		match.args...).Scan(&label)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("capture: reading the capturing mailbox's address: %w", err)
	}
	mailbox := foldAddress(bareAddress(label))
	return mailbox != "" && !slices.ContainsFunc(fields.Envelope.OpenRecipients, func(recipient string) bool {
		return foldAddress(recipient) != mailbox
	}), nil
}

// originalThreadTx is the thread the original belongs to: the seat's own copy
// of it when they hold one, else the root the forward's References name.
func originalThreadTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, original *connector.ForwardedOriginal) (string, error) {
	if original.MessageID == "" {
		return rec.ThreadKey, nil
	}
	var thread string
	err := tx.QueryRow(ctx, `
		SELECT coalesce(a.thread_key, '') FROM activity a
		  JOIN capture_import ci ON ci.activity_id = a.id AND ci.user_id = $3
		 WHERE a.source_system = $1 AND a.source_id = $2 AND a.archived_at IS NULL
		 LIMIT 1`, connector.EmailSourceSystem, original.MessageID, actorUserID(ctx)).Scan(&thread)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && thread == "") {
		return rec.ThreadKey, nil
	}
	if err != nil {
		return "", fmt.Errorf("capture: reading the forwarded original's thread: %w", err)
	}
	return thread, nil
}

// verdictThread is the thread whose verdicts judge rec. A forward to self is
// judged by its original's, so it is never more visible than the original.
func verdictThread(rec connector.NormalizedRecord) string {
	if fields, ok := rec.Fields.(ActivityFields); ok && fields.forwardedFrom != nil && fields.forwardThread != "" {
		return fields.forwardThread
	}
	return rec.ThreadKey
}

// verdictSender is whose address a thread verdict must have seen: the
// original's sender on a forward to self, else the counterparty.
func verdictSender(rec connector.NormalizedRecord) string {
	if original := forwardedOriginalOf(rec); original != nil {
		return original.From
	}
	return rec.Counterparty.Email
}

// ownAddressesOnlyTx reports mail whose every party is one of the acting seat's
// own addresses. A record whose parties cannot be read is not such mail.
func ownAddressesOnlyTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	if !partiesReadable(rec) {
		return false, nil
	}
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return !self.Empty() && len(self.WithoutSelf(rec.Addresses)) == 0, nil
}

// forwardedOriginalOf is the original of a forward to self, or nil.
func forwardedOriginalOf(rec connector.NormalizedRecord) *connector.ForwardedOriginal {
	if fields, ok := rec.Fields.(ActivityFields); ok {
		return fields.forwardedFrom
	}
	return nil
}

// withForwardOf is stored, a record read back from storage, carrying the
// vetted forward of captured, the record the capture read. Storage keeps no
// forward, so only the capture that vetted it can hand it on.
func withForwardOf(stored, captured connector.NormalizedRecord) connector.NormalizedRecord {
	fields, ok := stored.Fields.(ActivityFields)
	if original := forwardedOriginalOf(captured); ok && original != nil {
		fields.forwardedFrom = original
		stored.Fields = fields
	}
	return stored
}

// forwardIsFrom reports a forward to self whose original address sent.
func forwardIsFrom(fields ActivityFields, address string) bool {
	return fields.forwardedFrom != nil && normalizeEmail(fields.forwardedFrom.From) == normalizeEmail(address)
}

// forwardedSubject is the party a forward to self is filed under: the
// original's sender. The direction stays the message's own.
func forwardedSubject(original *connector.ForwardedOriginal, direction string) connector.Counterparty {
	return connector.Counterparty{
		Email:       original.From,
		DisplayName: original.DisplayName,
		Domain:      owndomain.OfAddress(original.From),
		Direction:   direction,
	}
}

// forwardPrefix opens the key one seat's forwards of one original share. It
// holds a space, which no Message-ID can, as calendarSendPrefix does.
const forwardPrefix = "forward "

// forwardKey is the identity of one original in one seat's forwards to self,
// or "". The original's sender and Message-ID name it; an inline forward has
// no Message-ID, so its quoted date and subject stand in. Hashed, because the forwarder
// chose its length.
func forwardKey(seat ids.UUID, fields ActivityFields) string {
	original := fields.forwardedFrom
	if seat == ids.Nil || original == nil {
		return ""
	}
	name := "id\x00" + original.From + "\x00" + original.MessageID
	if original.MessageID == "" {
		name = "inline\x00" + original.From + "\x00" + original.Date + "\x00" + original.Subject
	}
	sum := sha256.Sum256([]byte(name))
	return forwardPrefix + seat.String() + " " + hex.EncodeToString(sum[:])
}
