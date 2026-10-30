// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// transferOutcome is how a request ended for the colleague who asked, other
// than by being accepted.
type transferOutcome struct {
	kind          string
	subject, body langcopy.Phrase
}

var (
	transferDeclined = transferOutcome{"ownership_request_declined",
		ownershipTransferWords.Declined, ownershipTransferWords.DeclinedBody}
	transferExpired = transferOutcome{"ownership_request_expired",
		ownershipTransferWords.Expired, ownershipTransferWords.ExpiredBody}
	transferWithdrawn = transferOutcome{"ownership_request_withdrawn",
		ownershipTransferWords.Withdrawn, ownershipTransferWords.WithdrawnBody}
)

const transferAcceptedKind = "ownership_request_accepted"

// tellRequester gives the colleague who asked one line about how it ended.
func tellRequester(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, outcome transferOutcome) error {
	lang := identity.BaseLanguageForRecord(ctx, tx)
	owner, err := colleagueName(ctx, tx, transfer.FromID)
	if err != nil {
		return err
	}
	_, record, err := transferRecordTx(ctx, tx, transfer.RecordType, transfer.RecordID)
	if err != nil {
		return err
	}
	return recordTransferNotice(ctx, tx, approvalID, transfer, notices.NewNotice{
		Kind:    outcome.kind,
		Subject: fmt.Sprintf(outcome.subject.In(lang), owner, record),
		Body:    outcome.body.In(lang),
	})
}

// tellAccepted tells the receiver what is theirs now, and names what stayed
// with the giver because its own write refused the move.
func tellAccepted(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, moved transferHandOver) error {
	lang := identity.BaseLanguageForRecord(ctx, tx)
	giver, err := colleagueName(ctx, tx, transfer.FromID)
	if err != nil {
		return err
	}
	words := ownershipTransferWords
	lines := []string{fmt.Sprintf(words.AcceptedMoved.In(lang), moved.deals.Moved, moved.tasks.Moved)}
	if moved.deals.Kept > 0 {
		lines = append(lines, fmt.Sprintf(words.KeptDeals.In(lang), giver, moved.deals.Kept))
	}
	if moved.tasks.Kept > 0 {
		lines = append(lines, fmt.Sprintf(words.KeptTasks.In(lang), giver, moved.tasks.Kept))
	}
	return recordTransferNotice(ctx, tx, approvalID, transfer, notices.NewNotice{
		Kind:    transferAcceptedKind,
		Subject: fmt.Sprintf(words.Accepted.In(lang), giver, moved.record),
		Body:    strings.Join(lines, " "),
	})
}

// recordTransferNotice writes the line to the colleague who asked, about the
// record, once per request and outcome.
func recordTransferNotice(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, line notices.NewNotice) error {
	line.Recipient = ids.From[ids.UserKind](transfer.RequestedBy)
	line.Target = notices.Target{Type: transfer.RecordType, ID: transfer.RecordID}
	line.DedupeKey = fmt.Sprintf("%s:%s:%s", ownershipTransferKind, approvalID, line.Kind)
	_, err := notices.NewStore(nil).CreateTx(ctx, tx, line)
	return err
}

// transferRecords read the owner and the name of each record type a transfer
// moves.
var transferRecords = map[string]string{
	string(recordTypeContact): `SELECT owner_id, full_name FROM contact WHERE id = $1`,
	string(recordTypeCompany): `SELECT owner_id, display_name FROM company WHERE id = $1`,
}

// transferRecordTx answers the record's owner, nil for nobody, and its name.
func transferRecordTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (*ids.UUID, string, error) {
	statement, ok := transferRecords[recordType]
	if !ok {
		return nil, "", fmt.Errorf("compose: a %q cannot be handed over", recordType)
	}
	var owner *ids.UUID
	var name string
	err := tx.QueryRow(ctx, statement, id).Scan(&owner, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", apperrors.ErrNotFound
	}
	return owner, name, err
}

// retractApprovalNotice takes the decider's line about a request back once the
// request is gone.
func retractApprovalNotice(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) error {
	_, err := notices.NewStore(nil).Retract(ctx, tx, approvalNoticeDedupeKey(id.UUID))
	return err
}
