// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a private thread's files are allowed to leave behind.
//
// The read boundary already keeps a colleague out of personal mail, and
// erasure reaches the bytes. This is minimisation rather than correctness: not
// storing a payslip, a medical letter or a school form is strictly better than
// storing it and being able to delete it later, and a thread the classifier
// has already judged private is exactly the case where the product knows it
// does not want the file.
//
// Two verdicts make a message private: the thread's, and this seat's verdict
// that the SENDER is a personal correspondent. Either one keeps the files out
// of the object store and out of raw_capture, because a copy in the stored
// original is the same file kept somewhere else.
//
// The cost, stated rather than glossed: the owner loses the file LIST too. A
// row naming the attachment without its bytes would need the keeper contract
// to carry a withheld file, and that shape belongs to the module that owns the
// attachment table. Until then the breadcrumb is what says the files were kept
// out deliberately rather than never seen — which is the distinction somebody
// asking "where are my attachments" actually needs.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// withheldPersonalParts is the breadcrumb a stripped message leaves, in the
// same vocabulary a bounded one uses.
const withheldPersonalParts = "capture_personal_parts_withheld"

// verdictPersonalSender is the breadcrumb's verdict when the sender lane, not
// the thread, made the message private.
const verdictPersonalSender = "personal_sender"

// messageIsPrivateTx is the one answer to "does this message keep its files",
// asked before the raw store and again before staging, so the two cannot
// disagree about one message.
func messageIsPrivateTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (bool, string, error) {
	private, verdict, err := threadIsPrivateTx(ctx, tx, rec)
	if err != nil || private {
		return private, verdict, err
	}
	personal, err := senderIsPersonalTx(ctx, tx, rec, fields)
	if err != nil || !personal {
		return false, "", err
	}
	return true, verdictPersonalSender, nil
}

// senderIsPersonalTx reports whether this seat's settled verdict judged the
// sender a personal correspondent, and still reaches mail arriving now.
//
// It asks the sender-scoped half of PersonalPurgeScope (purgepersonal.go): the
// seat's own verdict, inbound mail only, inside noiseVerdictReach, and no
// `business` override and no attested reply to the address. The purge's other
// clauses are about a stored row — a hold, a contact filing — and no row exists
// yet. Spelled here rather than shared because that scope is a SQL fragment
// over a stored activity `a`, and this message is not one.
func senderIsPersonalTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields,
) (bool, error) {
	seat := actorUserID(ctx)
	address := normalizeEmail(rec.Counterparty.Email)
	if seat == ids.Nil || address == "" || fields.Kind != "email" || fields.Direction != "inbound" ||
		rec.Counterparty.SentByOwner() {
		return false, nil
	}
	var personal bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM capture_pending_counterparty p
		   WHERE p.owner_id = $1 AND p.email = $2
		     AND p.status = $3 AND p.kind = $4
		     AND p.resolved_at IS NOT NULL
		     AND now() <= p.resolved_at + `+quoteInterval(noiseVerdictReach)+`
		     AND NOT EXISTS (
		       SELECT 1 FROM capture_sender_override o
		        WHERE o.user_id = $1 AND o.address = $2 AND o.decision = $5)
		     AND NOT EXISTS (
		       SELECT 1 FROM activity c
		        WHERE c.counterparty_email = $2
		          AND c.direction = 'outbound' AND c.counterparty_outbound_attested))`,
		seat, address, PendingStatusNoise, KindPersonal, OverrideBusiness).Scan(&personal); err != nil {
		return false, fmt.Errorf("capture: reading whether this sender is a personal correspondent: %w", err)
	}
	return personal, nil
}

// withholdRawParts takes a private message's attachment bytes out of the
// original raw_capture is about to store. The parts themselves stay on the
// record until finishNewActivity strips them and leaves the breadcrumb.
func withholdRawParts(rec connector.NormalizedRecord) connector.NormalizedRecord {
	if len(rec.Parts) == 0 || len(rec.Raw) == 0 {
		return rec
	}
	parts := make([]partslim.WithheldPart, 0, len(rec.Parts))
	for _, part := range rec.Parts {
		parts = append(parts, partslim.WithheldPart{Ordinal: part.Ordinal, Body: part.Body})
	}
	rec.Raw, _ = partslim.WithholdParts(rec.Raw, parts)
	return rec
}

// fieldVerdict names WHICH act made the thread private: the classifier's
// reading, or the seat's own hand.
const fieldVerdict = "verdict"

// stripPersonalParts takes a private thread's files out of the record before
// anything stores them, and reports how many it withheld.
//
// The parts are REMOVED rather than emptied. Staging writes each body to the
// object store unconditionally, so an emptied part would put a zero-length
// object there — storing something, under a key the message then points at,
// which is worse than either keeping the file or keeping nothing.
//
// The count is what makes this answerable afterwards. A message that arrives
// with no parts and one whose parts were withheld look identical on the row;
// the breadcrumb is the only thing that separates "this had none" from "this
// had three and we deliberately kept none".
func stripPersonalParts(rec connector.NormalizedRecord) (connector.NormalizedRecord, int) {
	withheld := len(rec.Parts)
	rec.Parts = nil
	return rec, withheld
}

// personalPartsWithheld records that a thread's files were kept out, once per
// message and with no filename, type or size in it.
//
// A sender controls all three, and system_log is operational: the count is
// ours, the names are theirs.
//
// The VERDICT rides along because the two ways a thread becomes private are
// different acts. `held` is the classifier's reading, which somebody may
// disagree with; `held_by_owner` is the seat saying so by hand, which nobody
// overrules. A breadcrumb that credited the classifier for both would misstate
// half of them, and the half it misstates is the one where a human decided.
func (s *Sink) personalPartsWithheld(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, count int, verdict string,
) error {
	if count == 0 {
		return nil
	}
	return s.logBreadcrumbTx(ctx, tx, withheldPersonalParts, rec,
		"this thread is private",
		map[string]any{fieldDroppedParts: count, fieldVerdict: verdict})
}
