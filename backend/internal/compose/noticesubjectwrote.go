// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A contact who writes to us owes no Art. 14 notice, whichever order a
// backfill read the conversation in.
//
// contacts decides a captured contact's acquisition once, when it mints the
// row: subject_initiated when a mail FROM the address is already captured,
// unknown_legacy otherwise. A mailbox backfill does not read oldest first, so
// it often mints the contact from our own reply, and the contact's first mail
// arrives minutes later — after the case was opened. This is the other half:
// that later mail records the acquisition it evidences and settles the case.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fromSubjectMail is the rule for "a mail they sent us": an inbound, live mail
// whose From is the address, not bulk — a newsletter is a list writing to
// everyone — and DELIVERED BY A CONNECTOR. The last term is the trust line. A
// mail a seat logs by hand also announces activity.captured, and its From is
// whatever the seat typed; counting it would let anybody close a legal duty by
// logging a message "from" the contact. A connector's mail came out of a real
// mailbox. Its From header is still the sender's own claim, which is the same
// evidence contacts decides a new contact's acquisition from; authenticating it
// is a question for both rules at once.
const fromSubjectMail = `
	a.kind = 'email' AND a.direction = 'inbound'
	AND a.archived_at IS NULL AND NOT a.bulk_mail_attested
	AND starts_with(a.captured_by, 'connector:')`

// capturedAcquisition is the provenance of an acquisition capture wrote. Only
// those are settled by a later mail: an unknown source a seat or an import
// stated is a claim about somewhere else, and the contact writing to us later
// says nothing about where that address came from.
const capturedAcquisition = ` starts_with(e.captured_by, 'connector:')`

// contactWroteToUsTx reports that a mail from one of this contact's addresses
// has been captured.
//
// lower(p.address) on the participant side, because that is what the address
// index covers; contact addresses are stored lowercase.
func contactWroteToUsTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (bool, error) {
	var wrote bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM contact_email ce
		    JOIN activity_participant p
		      ON lower(p.address) = ce.email AND p.address IS NOT NULL AND p.role = 'from'
		    JOIN activity a ON a.id = p.activity_id
		   WHERE ce.contact_id = $1 AND ce.archived_at IS NULL AND`+fromSubjectMail+`)`,
		contactID).Scan(&wrote); err != nil {
		return false, fmt.Errorf("did this contact write to us: %w", err)
	}
	return wrote, nil
}

// lockAcquisitionsTx takes the contact's acquisition rows FOR UPDATE.
//
// The opener and the settler both take it first, which is what orders them. The
// opener decides from "has their mail been captured" and then inserts a case;
// the settler looks for that case. Without a shared lock the opener could read
// no mail, the settler find no case, and the case the opener then inserts would
// stay open for good. With it, whichever runs second sees what the first
// committed.
func lockAcquisitionsTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) error {
	if _, err := tx.Exec(ctx, `
		SELECT 1 FROM contact_acquisition_evidence WHERE contact_id = $1 FOR UPDATE`,
		contactID); err != nil {
		return fmt.Errorf("lock the acquisitions this contact arrived by: %w", err)
	}
	return nil
}

// settleWhenSubjectWrote reads one captured activity and, when it is a mail a
// contact sent us, settles the duties that contact was owed only because
// capture could not say where their address came from.
//
// Per contact it writes a subject_initiated acquisition naming the mail — the
// evidence the closed case rests on — and then closes that contact's cases on
// capture's own unknown_legacy acquisitions. A duty opened for a referral, a
// bought list, or an unknown source somebody else stated is not answered by the
// contact writing later.
//
// A redelivered activity.captured finds the cases already closed and the
// acquisition already there, and does nothing.
func (n *NoticeCaseOpen) settleWhenSubjectWrote(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ce.contact_id, a.occurred_at
		  FROM activity a
		  JOIN activity_participant p
		    ON p.activity_id = a.id AND p.role = 'from' AND p.address IS NOT NULL
		  JOIN contact_email ce ON ce.email = lower(p.address) AND ce.archived_at IS NULL
		 WHERE a.id = $1 AND`+fromSubjectMail, activityID)
	if err != nil {
		return fmt.Errorf("read the contacts this mail came from: %w", err)
	}
	type sender struct {
		contact ids.UUID
		sent    time.Time
	}
	senders, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (sender, error) {
		var s sender
		err := r.Scan(&s.contact, &s.sent)
		return s, err
	})
	if err != nil {
		return fmt.Errorf("read the contacts this mail came from: %w", err)
	}
	for _, s := range senders {
		if err := n.settleSender(ctx, tx, s.contact, activityID, s.sent); err != nil {
			return err
		}
	}
	return nil
}

// settleSender settles one contact's capture-unknown duties on one mail of theirs.
func (n *NoticeCaseOpen) settleSender(ctx context.Context, tx pgx.Tx, contactID, mail ids.UUID, sent time.Time) error {
	if err := lockAcquisitionsTx(ctx, tx, contactID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT e.id FROM contact_acquisition_evidence e
		 WHERE e.contact_id = $1 AND e.kind = $2 AND`+capturedAcquisition,
		contactID, contacts.AcquiredUnknownLegacy)
	if err != nil {
		return fmt.Errorf("read what capture could not source: %w", err)
	}
	unknown, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("read what capture could not source: %w", err)
	}
	if len(unknown) == 0 {
		return nil
	}
	if err := contacts.RecordSubjectWroteTx(ctx, tx, ids.From[ids.ContactKind](contactID), mail, sent, systemNoticeCaseActor); err != nil {
		return err
	}
	closed, err := consent.SettleWhenSubjectWroteTx(ctx, tx, unknown, n.now())
	if err != nil {
		return err
	}
	if closed > 0 {
		n.log.InfoContext(ctx, "a contact's own mail settled their notice duty",
			slog.String("contact_id", contactID.String()), slog.Int("closed", closed))
	}
	return nil
}
