// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a caller says about WHY they are writing, decoded off the wire.
//
// The four fields travel together because they answer one question between
// them — the category claimed, the topic a marketing send is for, the sentence
// a human typed about an ambiguous first message, and the records offered in
// support. Splitting them across a parameter list would put four more
// positional arguments on a decoder that already carries nine.
//
// None of it authorizes anything. A claim is a claim: the engine reads the
// evidence, resolves the category the evidence actually supports, and records
// both, so a caller naming `invoice_or_payment` over a message with no invoice
// behind it produces a visible disagreement rather than a send.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// contextField is the wire path a refusal about a claimed category points at,
// and operatorReasonField its twin for the sentence beside it.
const (
	contextField        = "communication_context"
	operatorReasonField = "operator_reason"
)

// maxOperatorReasonRunes mirrors the contract's maxLength. Named rather than
// inlined because the refusal message states it too, and two spellings of one
// bound drift.
const maxOperatorReasonRunes = 500

// sendContext is the decoded claim.
type sendContext struct {
	category  commsauthz.Category
	marketing string
	reason    string
	evidence  commsauthz.Evidence
}

// applyTo puts the claim on the send input.
func (c sendContext) applyTo(in SendEmailInput) SendEmailInput {
	in.Context = c.category
	in.MarketingPurpose = c.marketing
	in.OperatorReason = c.reason
	in.Evidence = c.evidence
	return in
}

// sendContextFrom decodes and validates what the caller claimed.
//
// An unknown category is refused rather than dropped: a caller who misspells
// one and is silently given the engine's own resolution has been told their
// claim was accepted when it was never read.
func sendContextFrom(category *string, marketing, reason *string, evidence *crmcontracts.CommunicationEvidence) (sendContext, error) {
	out := sendContext{
		marketing: deref(marketing),
		reason:    deref(reason),
		evidence:  evidenceFrom(evidence),
	}
	// The contract bounds the reason at 500 and nothing in this stack validates
	// a request against the schema, so the bound is enforced here or nowhere. A
	// sentence about one send is short; anything longer is a paste accident or
	// an attempt to use the decision trail as storage.
	if len([]rune(out.reason)) > maxOperatorReasonRunes {
		return sendContext{}, &CommunicationContextError{
			field:  operatorReasonField,
			Reason: "an operator reason is at most 500 characters",
		}
	}
	if category == nil || *category == "" {
		return out, nil
	}
	claimed := commsauthz.Category(*category)
	if !claimed.Valid() {
		return sendContext{}, &CommunicationContextError{
			Reason: "that is not a communication category",
		}
	}
	// The five categories that exist to serve the recipient are the ones a
	// hard suppression does not stop, and they are reserved for the
	// installation's own controller mail, which rides a registered template. A
	// caller able to claim one could dress marketing as a security warning and
	// reach somebody who has objected — so the claim is refused here, at the
	// door, rather than left for the engine to disbelieve.
	if claimed.ServesTheSubject() {
		return sendContext{}, &CommunicationContextError{
			Reason: "that category is reserved for the installation's own notices and cannot be claimed by a send",
		}
	}
	out.category = claimed
	return out, nil
}

// evidenceFrom flattens the optional block. An absent id stays zero, which is
// what the engine reads as "the caller named none".
func evidenceFrom(in *crmcontracts.CommunicationEvidence) commsauthz.Evidence {
	if in == nil {
		return commsauthz.Evidence{}
	}
	return commsauthz.Evidence{
		ActivityID:     derefID(in.ActivityId),
		DealID:         derefID(in.DealId),
		InvoiceID:      derefID(in.InvoiceId),
		ContractID:     derefID(in.ContractId),
		ConsentEventID: derefID(in.ConsentEventId),
		BasisID:        derefID(in.BasisId),
	}
}

func derefID(id *openapi_types.UUID) ids.UUID {
	if id == nil {
		return ids.UUID{}
	}
	return ids.UUID(*id)
}

// CommunicationContextError maps to 422: the caller named a category, and it is
// not one they may name. The claimed value is never echoed back — the wire's own
// field pointer says which input to change, and the message says why.
type CommunicationContextError struct {
	// field is the wire path the refusal points at, so a caller is told which
	// of the four inputs to change rather than being handed the block's name.
	field  string
	Reason string
}

func (e *CommunicationContextError) Error() string { return e.Reason }

// FieldFault names the offending field, on every surface.
func (e *CommunicationContextError) FieldFault() (field, code, message string) {
	named := e.field
	if named == "" {
		named = contextField
	}
	return named, faultInvalid, e.Reason
}

// replySendInput decodes a reply's whole body into the send input: the message
// itself, and the claim the caller made about why they are writing.
//
// Here rather than in the handler because the two are one decode. Splitting the
// claim from the message put both halves in a handler that had already outgrown
// the file cap, and neither half means anything without the other.
func replySendInput(req crmcontracts.SendEmailRequest) (SendEmailInput, error) {
	claimed, err := sendContextFrom((*string)(req.CommunicationContext),
		req.MarketingPurpose, req.OperatorReason, req.Evidence)
	if err != nil {
		return SendEmailInput{}, err
	}
	return claimed.applyTo(sendInputFrom(
		req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.HtmlBody, req.AttachmentIds,
		req.ConsentPurpose, req.DraftRef,
	)), nil
}

// accountSendInput is the same decode for an account-started send, which names
// its own links and has no anchor to derive a category from — the one shape
// where a caller's claim is the only thing that can say what the message is.
func accountSendInput(req crmcontracts.SendAccountEmailRequest) (SendEmailInput, error) {
	claimed, err := sendContextFrom((*string)(req.CommunicationContext),
		req.MarketingPurpose, req.OperatorReason, req.Evidence)
	if err != nil {
		return SendEmailInput{}, err
	}
	return claimed.applyTo(sendInputFrom(
		req.To, req.Cc, req.Bcc, req.Subject, req.Body, req.HtmlBody, req.AttachmentIds,
		req.ConsentPurpose, req.DraftRef,
	)), nil
}
