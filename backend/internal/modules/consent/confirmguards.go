// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What a confirm link refuses to ask, and of whom.
//
// Both guards run inside issueLink's own transaction, after the destination
// address is derived and before the token row is written, so a refusal mints
// nothing and mails nothing. They are here rather than beside the mint because
// they answer a different question from it: the mint knows HOW to make a link,
// and these decide WHETHER this contact should be asked at all.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkRequest is what the guards need to judge one mint.
type linkRequest struct {
	contactID       ids.ContactID
	purposeID       ids.PurposeID
	expectedAddress string
}

// admitLinkTx runs every check a mint owes before it writes anything, and
// returns the address the link must be delivered to.
//
// They are one call because they are one decision — may this contact be sent
// this question, and where — and because the ORDER matters. The subject is held
// live FIRST, before any other row lock: the same ordering the erasure path
// takes, so an erasure committing after an unheld probe cannot leave the
// installation posting a link to somebody it was just told to forget. The
// address is derived before the last two guards because both judge it.
func admitLinkTx(ctx context.Context, tx pgx.Tx, req linkRequest) (string, error) {
	if err := auth.HoldWritableLive(ctx, tx, "contact", req.contactID.UUID); err != nil {
		return "", err
	}
	if err := requireConfirmablePurposeTx(ctx, tx, req.purposeID); err != nil {
		return "", err
	}
	deliveredTo, err := deliveryAddressTx(ctx, tx, req.contactID)
	if err != nil {
		return "", err
	}
	if err := requireExpectedAddress(req.expectedAddress, deliveredTo); err != nil {
		return "", err
	}
	if err := refuseWithdrawnPurposeTx(ctx, tx, req.contactID, req.purposeID); err != nil {
		return "", err
	}
	return deliveredTo, nil
}

// requireExpectedAddress refuses a mint whose destination is not the address the
// requester named. An empty expectation asks nothing: the caller named a contact
// and never claimed which mailbox that is.
//
// The comparison is case-insensitive because the lookup that resolved the contact
// was — an address differing only in case is the SAME mailbox and must not read
// as a mismatch. Both sides are trimmed. The stored side is written normalized
// today, so trimming it changes nothing now; a guard that refuses a real match
// on stray whitespace is the failure that would be hard to recognise later, and
// it costs nothing to be symmetric.
//
// The refusal names neither address. This runs behind an anonymous door, so
// saying "we will send to v...r@example.com instead" would turn the mint into an
// oracle for the addresses a contact holds.
func requireExpectedAddress(expected, deliveredTo string) error {
	if expected == "" || strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(deliveredTo)) {
		return nil
	}
	return &MisdirectedLinkError{}
}

// MisdirectedLinkError refuses a mint whose link would reach a mailbox other
// than the one that asked.
//
// A DISTINCT TYPE, because a booking form has to tell this refusal apart from
// the others this mint makes. The rest — no live address on the record, a
// purpose archived under a live form — say this installation cannot put the
// question, and a booking must survive them: the tick was optional and the
// meeting was not. This one says the question would go to the WRONG CONTACT,
// which is not a question worth asking at any price.
type MisdirectedLinkError struct{}

func (e *MisdirectedLinkError) Error() string {
	return "this address is on file for a contact whose confirmations go elsewhere, " +
		"so the link would reach a different mailbox than the one that asked"
}

// FieldFault carries the refusal to every surface, the field naming the contact
// because that is the record whose addresses disagree.
func (e *MisdirectedLinkError) FieldFault() (field, code, message string) {
	return contactIDKey, "confirmations_go_elsewhere", e.Error()
}

// refuseWithdrawnPurposeTx refuses to ask again about a purpose the subject has
// already taken back. A record-confirmation link names no purpose and is exempt.
//
// It lives HERE rather than at the booking edge because both doors need it and
// the anonymous one is not the only way to reach a withdrawn subject: an
// operator can press the double-opt-in verb on the same contact just as easily.
//
// Nothing downstream stops this mail. The confirmation template's category
// serves the subject, which is exactly the class the withdrawal validator lets
// through — rightly, because an unsubscribe acknowledgement must reach somebody
// who just unsubscribed. A fresh invitation to resubscribe is the opposite
// message wearing that exemption, so it has to be refused before it is staged.
//
// Verified rather than reasoned: before this guard, a booking naming a
// withdrawn subject answered 201 and minted a link.
//
// A withdrawal is not permanent for the SUBJECT — they may re-subscribe through
// the preference centre, which is their own mailbox and their own choice. What
// is refused is somebody ELSE restarting the conversation on their behalf.
func refuseWithdrawnPurposeTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, purposeID ids.PurposeID) error {
	if purposeID.UUID == (ids.UUID{}) {
		return nil
	}
	var state string
	err := tx.QueryRow(ctx,
		`SELECT state FROM contact_consent WHERE contact_id = $1 AND purpose_id = $2`,
		contactID, purposeID).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if ConsentState(state) != StateWithdrawn {
		return nil
	}
	return &ReSolicitationError{}
}

// ReSolicitationError refuses asking again about a purpose the subject has
// already taken back.
//
// A distinct type for MisdirectedLinkError's reason, and the stronger case of
// the two: a withdrawal is the subject's own instruction, and a booking form
// that swallowed this refusal would mail the newsletter question to somebody
// who explicitly said stop — the exact act the withdrawal forbids.
type ReSolicitationError struct{}

func (e *ReSolicitationError) Error() string {
	return "this contact has withdrawn this purpose, so we do not ask them about it again"
}

// FieldFault carries the refusal to every surface, the field naming the purpose
// because that is what the caller would change.
func (e *ReSolicitationError) FieldFault() (field, code, message string) {
	return purposeIDField, "purpose_withdrawn", e.Error()
}
