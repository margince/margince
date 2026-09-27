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

// fromSubjectMail is the rule for "a mail they sent us", the same one contacts
// applies when it mints the row (acquiredFromCaptureTx): an inbound, live mail
// whose From is the address, and not bulk — a newsletter is a list writing to
// everyone, not the sender writing to us.
const fromSubjectMail = `
	a.kind = 'email' AND a.direction = 'inbound'
	AND a.archived_at IS NULL AND NOT a.bulk_mail_attested`

// contactWroteToUsTx reports that a mail from one of this contact's addresses
// has been captured.
func contactWroteToUsTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (bool, error) {
	var wrote bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM contact_email ce
		    JOIN activity_participant p ON p.address = ce.email AND p.role = 'from'
		    JOIN activity a ON a.id = p.activity_id
		   WHERE ce.contact_id = $1 AND ce.archived_at IS NULL AND`+fromSubjectMail+`)`, contactID).Scan(&wrote); err != nil {
		return false, fmt.Errorf("did this contact write to us: %w", err)
	}
	return wrote, nil
}

// settleWhenSubjectWrote reads one captured activity and, when it is a mail a
// contact sent us, settles the duties that contact was owed only because their
// source was unknown.
//
// Per contact it writes a subject_initiated acquisition dated from the mail —
// the evidence the closed case rests on, readable where every other
// acquisition is — and then closes that contact's cases on unknown_legacy
// acquisitions. Only those: a duty opened for a referral or a bought list is
// not answered by the subject writing later, because the address still came
// from somebody else.
//
// A redelivered activity.captured finds the cases already closed and the
// acquisition already there, and does nothing.
func (n *NoticeCaseOpen) settleWhenSubjectWrote(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT e.contact_id, array_agg(e.id), min(a.occurred_at)
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
		  JOIN contact_email ce ON ce.email = p.address AND ce.archived_at IS NULL
		  JOIN contact_acquisition_evidence e
		    ON e.contact_id = ce.contact_id AND e.kind = $2
		 WHERE a.id = $1 AND`+fromSubjectMail+`
		 GROUP BY e.contact_id`, activityID, contacts.AcquiredUnknownLegacy)
	if err != nil {
		return fmt.Errorf("read the contacts this mail came from: %w", err)
	}
	type sender struct {
		contact      ids.UUID
		acquisitions []ids.UUID
		sent         time.Time
	}
	senders, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (sender, error) {
		var s sender
		err := r.Scan(&s.contact, &s.acquisitions, &s.sent)
		return s, err
	})
	if err != nil {
		return fmt.Errorf("read the contacts this mail came from: %w", err)
	}
	for _, s := range senders {
		if err := contacts.RecordSubjectWroteTx(ctx, tx, ids.From[ids.ContactKind](s.contact), s.sent, systemNoticeCaseActor); err != nil {
			return err
		}
		closed, err := consent.SettleWhenSubjectWroteTx(ctx, tx, s.acquisitions, n.now())
		if err != nil {
			return err
		}
		if closed > 0 {
			n.log.InfoContext(ctx, "a contact's own mail settled their notice duty",
				slog.String("contact_id", s.contact.String()), slog.Int("closed", closed))
		}
	}
	return nil
}
