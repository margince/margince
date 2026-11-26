// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/recordowner"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// transferOutcome is how a request ended for the colleague who asked, other
// than by being accepted.
type transferOutcome struct {
	kind                    string
	subject, body           langcopy.Phrase
	offerSubject, offerBody langcopy.Phrase
	code                    string
}

var (
	transferDeclined = transferOutcome{
		"ownership_request_declined",
		ownershipTransferWords.Declined, ownershipTransferWords.DeclinedBody,
		ownershipTransferWords.OfferDeclined, ownershipTransferWords.OfferDeclinedBody,
		"Declined",
	}
	transferExpired = transferOutcome{
		"ownership_request_expired",
		ownershipTransferWords.Expired, ownershipTransferWords.ExpiredBody,
		ownershipTransferWords.OfferExpired, ownershipTransferWords.OfferExpiredBody,
		"Expired",
	}
	transferWithdrawn = transferOutcome{
		"ownership_request_withdrawn",
		ownershipTransferWords.Withdrawn, ownershipTransferWords.WithdrawnBody,
		ownershipTransferWords.OfferWithdrawn, ownershipTransferWords.OfferWithdrawnBody,
		"Withdrawn",
	}
)

const transferAcceptedKind = "ownership_request_accepted"

// tellRequester gives the colleague who staged the transfer one line about
// how it ended, naming the colleague who was to answer it.
func tellRequester(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, outcome transferOutcome) error {
	lang := identity.BaseLanguageForRecord(ctx, tx)
	code := "ownership." + outcome.code
	subject, body := outcome.subject, outcome.body
	if transfer.Offered() {
		subject, body = outcome.offerSubject, outcome.offerBody
		code = "ownership.Offer" + outcome.code
	}
	other, err := recordowner.ColleagueNameTx(ctx, tx, transfer.DeciderID)
	if err != nil {
		return err
	}
	record, err := recordowner.NameForTx(ctx, tx, transfer.RequestedBy, transfer.RecordType, transfer.RecordID, lang)
	if err != nil {
		return err
	}
	return recordTransferNotice(ctx, tx, approvalID, transfer, notices.NewNotice{
		Kind:       outcome.kind,
		SystemText: &systemtext.Message{Version: 1, Subject: systemtext.Sentence(code, systemtext.Label(other), systemtext.Label(record)), Body: systemtext.Sentence(code + "Body")},
		Subject:    fmt.Sprintf(subject.In(lang), other, record),
		Body:       body.In(lang),
	})
}

// tellAccepted tells the colleague who staged the transfer what moved, and
// names what stayed with the giver because its own write refused the move.
func tellAccepted(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, moved transferHandOver) error {
	lang := identity.BaseLanguageForRecord(ctx, tx)
	giver, err := recordowner.ColleagueNameTx(ctx, tx, transfer.FromID)
	if err != nil {
		return err
	}
	words := ownershipTransferWords
	subject, movedLine := fmt.Sprintf(words.Accepted.In(lang), giver, moved.record), words.AcceptedMoved
	titleMetadata := systemtext.Sentence("ownership.Accepted", systemtext.Label(giver), systemtext.Text(moved.record))
	bodyCode := "ownership.AcceptedMoved"
	if transfer.Offered() {
		receiver, err := recordowner.ColleagueNameTx(ctx, tx, transfer.ToID)
		if err != nil {
			return err
		}
		subject, movedLine = fmt.Sprintf(words.OfferAccepted.In(lang), receiver, moved.record), words.OfferAcceptedMoved
		titleMetadata = systemtext.Sentence("ownership.OfferAccepted", systemtext.Text(receiver), systemtext.Text(moved.record))
		bodyCode = "ownership.OfferAcceptedMoved"
	}
	lines := []string{fmt.Sprintf(movedLine.In(lang), moved.deals.Moved, moved.tasks.Moved)}
	bodyMetadata := systemtext.Sentence(bodyCode, systemtext.Count(int64(moved.deals.Moved)), systemtext.Count(int64(moved.tasks.Moved)))
	if moved.deals.Kept > 0 {
		lines = append(lines, fmt.Sprintf(words.KeptDeals.In(lang), giver, moved.deals.Kept))
		bodyMetadata = systemtext.Sequence(bodyMetadata, systemtext.Sentence("ownership.KeptDeals", systemtext.Label(giver), systemtext.Count(int64(moved.deals.Kept))))
	}
	if moved.tasks.Kept > 0 {
		lines = append(lines, fmt.Sprintf(words.KeptTasks.In(lang), giver, moved.tasks.Kept))
		bodyMetadata = systemtext.Sequence(bodyMetadata, systemtext.Sentence("ownership.KeptTasks", systemtext.Label(giver), systemtext.Count(int64(moved.tasks.Kept))))
	}
	return recordTransferNotice(ctx, tx, approvalID, transfer, notices.NewNotice{
		Kind:       transferAcceptedKind,
		SystemText: &systemtext.Message{Version: 1, Subject: titleMetadata, Body: bodyMetadata},
		Subject:    subject,
		Body:       strings.Join(lines, " "),
	})
}

// recordTransferNotice writes the line to the colleague who staged the
// transfer, about the record, once per transfer and outcome.
func recordTransferNotice(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, transfer approvalsubject.OwnershipTransfer, line notices.NewNotice) error {
	line.Recipient = ids.From[ids.UserKind](transfer.RequestedBy)
	line.Target = notices.Target{Type: transfer.RecordType, ID: transfer.RecordID}
	line.DedupeKey = fmt.Sprintf("%s:%s:%s", ownershipTransferKind, approvalID, line.Kind)
	_, err := notices.NewStore(nil).CreateTx(ctx, tx, line)
	return err
}

// transferRecords are the record types a transfer moves.
var transferRecords = map[string]bool{string(recordTypeContact): true, string(recordTypeCompany): true}

// transferRecordTx answers the record's owner, nil for nobody, and its name.
func transferRecordTx(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) (*ids.UUID, string, error) {
	if !transferRecords[recordType] {
		return nil, "", fmt.Errorf("compose: a %q cannot be handed over", recordType)
	}
	return recordowner.ReadTx(ctx, tx, recordType, id)
}

// retractApprovalNotice takes the decider's line about a request back once the
// request is gone.
func retractApprovalNotice(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) error {
	_, err := notices.NewStore(nil).Retract(ctx, tx, approvalNoticeDedupeKey(id.UUID))
	return err
}
