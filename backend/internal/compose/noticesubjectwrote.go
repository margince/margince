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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// capturedAcquisition identifies provenance for the historical capture-only repairs.
var capturedAcquisition = correspondence.CapturedAcquisitionSQL("e")

// contactWroteToUsTx reports that a mail this contact wrote has been captured.
func contactWroteToUsTx(ctx context.Context, tx pgx.Tx, contactID ids.UUID) (bool, error) {
	var wrote bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+consent.SubjectReachedOutFromSQL("$1::uuid")+`)`,
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

// settleWhenSubjectWrote closes all still-owed sources when a mail or meeting
// proves the contact approached us. Replays find no open cases to change.
func (n *NoticeCaseOpen) settleWhenSubjectWrote(ctx context.Context, tx pgx.Tx, activityID ids.UUID) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT sender.contact_id, `+consent.SubjectReachedOutAtSQL("a")+` AS reached_at
		  FROM activity a
		  JOIN activity_participant p ON p.activity_id = a.id
		 CROSS JOIN LATERAL `+relstrength.ParticipantContactsSQL("p")+` sender(contact_id)
		 WHERE a.id = $1 AND`+consent.SubjectReachedOutSQL("a", "sender.contact_id")+` ORDER BY sender.contact_id, reached_at`, activityID)
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

// settleSender records one contact’s own approach and closes their open duties.
func (n *NoticeCaseOpen) settleSender(ctx context.Context, tx pgx.Tx, contactID, mail ids.UUID, sent time.Time) error {
	closed, err := settleSenderCasesTx(ctx, tx, contactID, mail, sent, n.now())
	if err != nil {
		return err
	}
	if closed > 0 {
		n.log.InfoContext(ctx, "a contact's own mail settled their notice duty", slog.String("contact_id", contactID.String()), slog.Int("closed", closed))
	}
	return nil
}

func settleSenderCasesTx(ctx context.Context, tx pgx.Tx, contactID, mail ids.UUID, sent, now time.Time) (int, error) {
	// Match the opener's lock order: contact, acquisitions, then cases. Recording
	// the new proof takes a foreign-key lock on the contact too.
	var held ids.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM contact WHERE id = $1 AND archived_at IS NULL FOR UPDATE`, contactID).Scan(&held); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if err := lockAcquisitionsTx(ctx, tx, contactID); err != nil {
		return 0, err
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND `+consent.SubjectReachedOutSQL("a", "$2::uuid")+`)`, mail, contactID).Scan(&current); err != nil {
		return 0, err
	}
	if !current {
		return 0, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT e.id FROM contact_acquisition_evidence e
		 WHERE e.contact_id = $1`, contactID)
	if err != nil {
		return 0, fmt.Errorf("read what capture could not source: %w", err)
	}
	unknown, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return 0, fmt.Errorf("read what capture could not source: %w", err)
	}
	if len(unknown) == 0 {
		return 0, nil
	}
	if err := contacts.RecordSubjectWroteTx(ctx, tx, ids.From[ids.ContactKind](contactID), mail, sent, systemNoticeCaseActor); err != nil {
		return 0, err
	}
	closed, err := consent.SettleWhenSubjectReachedOutTx(ctx, tx, unknown, mail, now)
	if err != nil {
		return 0, err
	}
	return closed, nil
}

// settleContactApproach is reached after merges and address changes, when
// earlier correspondence may now belong to this contact.
func (n *NoticeCaseOpen) settleContactApproach(ctx context.Context, tx pgx.Tx, contact ids.UUID) error {
	var activity ids.UUID
	var at time.Time
	err := tx.QueryRow(ctx, `SELECT a.id, `+consent.SubjectReachedOutAtSQL("a")+` FROM `+consent.SubjectReachedOutFromSQL("$1::uuid")+` ORDER BY `+consent.SubjectReachedOutAtSQL("a")+`, a.id LIMIT 1`, contact).Scan(&activity, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return n.settleSender(ctx, tx, contact, activity, at)
}
