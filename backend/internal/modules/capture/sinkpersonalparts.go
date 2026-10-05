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
// The owner keeps the file LIST: each withheld file is recorded by name, size
// and type on a row with no bytes behind it (FileKeeper.RecordWithheld), so
// "where are my attachments" has an answer on the message itself.

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

// messageIsPrivateTx answers "does this message keep its files". The raw store
// and staging both call it with the same record.
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
// `business` override and no attested reply to the address. It also refuses an
// address a correspondence contact holds, because unlike the purge it acts at
// once and leaves no window to correct a forged verdict. The purge's other
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
	// Locked, through the reader the verdict engine uses: a `business` override
	// committing while this message is captured must win, not lose its files.
	override, err := OverrideForTx(ctx, tx, seat, address)
	if err != nil || override == OverrideBusiness {
		return false, err
	}
	// The From header is unauthenticated, so a forged message can earn an
	// address a personal verdict. A contact the workspace corresponds with at
	// that address outranks it, exactly as it does for noiseMailScope.
	var personal bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM capture_pending_counterparty p
		   WHERE p.owner_id = $1 AND p.email = $2
		     AND p.status = $3 AND p.kind = $4
		     AND p.resolved_at IS NOT NULL
		     AND now() <= p.resolved_at + `+quoteInterval(noiseVerdictReach)+`
		     AND NOT EXISTS (
		       SELECT 1 FROM activity c
		        WHERE c.counterparty_email = $2
		          AND c.direction = 'outbound' AND c.counterparty_outbound_attested
		          AND c.archived_at IS NULL)
		     AND NOT EXISTS (
		       SELECT 1 FROM contact_email pe JOIN contact pr ON pr.id = pe.contact_id
		        WHERE pe.email = $2 AND pr.archived_at IS NULL AND pe.archived_at IS NULL
		          AND pe.from_correspondence))`,
		seat, address, PendingStatusNoise, KindPersonal).Scan(&personal); err != nil {
		return false, fmt.Errorf("capture: reading whether this sender is a personal correspondent: %w", err)
	}
	return personal, nil
}

// storeOriginalTx writes the record's original to raw_capture and stamps the
// stored row back onto the record, so the activity can name the original it was
// read from. Raw capture is append-once, so a private message's files leave the
// original here, before its only write.
func storeOriginalTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (connector.NormalizedRecord, error) {
	rec, err := withholdPrivateOriginalTx(ctx, tx, rec)
	if err != nil {
		return rec, err
	}
	stored, err := storeRawCapture(ctx, tx, rec)
	if err != nil {
		return rec, err
	}
	rec.StoredOriginal = stored
	return rec, nil
}

// withholdPrivateOriginalTx is the record raw_capture should store: a private
// message's original without its attachment bytes, anything else unchanged.
func withholdPrivateOriginalTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord,
) (connector.NormalizedRecord, error) {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok {
		return rec, nil
	}
	private, _, err := messageIsPrivateTx(ctx, tx, rec, fields)
	if err != nil || !private {
		return rec, err
	}
	return withholdRawParts(rec), nil
}

// withholdRawParts takes a private message's attachment bytes out of the
// original raw_capture is about to store. The parts themselves stay on the
// record until finishNewActivity strips them and leaves the breadcrumb.
func withholdRawParts(rec connector.NormalizedRecord) connector.NormalizedRecord {
	if len(rec.Raw) == 0 || (len(rec.Parts) == 0 && len(rec.PartDrops) == 0) {
		return rec
	}
	// A file the bounds refused is in no part to splice, yet still in the
	// original, so only its headers are kept.
	if len(rec.PartDrops) > 0 {
		rec.Raw = partslim.HeadersOnly(rec.Raw)
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
// anything stores them, and reports how many it withheld. The caller keeps the
// parts it took, to name them on rows without bytes.
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
