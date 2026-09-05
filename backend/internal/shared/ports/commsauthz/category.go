// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package commsauthz is the vocabulary an outbound authorization decision is
// written in: what kind of message this is, what makes it lawful, and what the
// engine answered.
//
// It sits in shared/ports because three modules speak it and none may import
// another: activities builds the question, comms carries it on the delivery
// row, and consent answers it. The types are data — no behaviour reaches back
// into a module.
package commsauthz

// Category is what kind of communication this is. Closed and server-resolved:
// a caller may CLAIM one, and the engine decides which one the evidence
// actually supports. There is deliberately no generic "transactional" member —
// that was the escape hatch that let any message call itself operational.
type Category string

const (
	// CategoryReplyToInbound answers a message the subject sent.
	CategoryReplyToInbound Category = "reply_to_inbound"
	// CategoryRequestedFollowup continues something the subject asked for.
	CategoryRequestedFollowup Category = "requested_followup"
	// CategoryPrecontractQuote carries a quote or offer that was requested.
	CategoryPrecontractQuote Category = "precontract_quote"
	// CategoryActiveDealFollowup moves a live opportunity along.
	CategoryActiveDealFollowup Category = "active_deal_followup"
	// CategoryCustomerService answers a service case or request.
	CategoryCustomerService Category = "customer_service"
	// CategoryAccountNotice reports an account event to its holder.
	CategoryAccountNotice Category = "account_notice"
	// CategoryContractNotice carries a notice a live contract requires.
	CategoryContractNotice Category = "contract_notice"
	// CategoryInvoiceOrPayment carries a named financial event.
	CategoryInvoiceOrPayment Category = "invoice_or_payment"
	// CategorySecurityNotice reports a security event affecting the subject.
	CategorySecurityNotice Category = "security_notice"
	// CategoryPrivacyNotice discharges an information duty.
	CategoryPrivacyNotice Category = "privacy_notice"
	// CategoryRecordConfirmation asks the subject to check what is held.
	CategoryRecordConfirmation Category = "record_confirmation"
	// CategoryConsentConfirmation carries the double-opt-in link.
	CategoryConsentConfirmation Category = "consent_confirmation"
	// CategoryOptoutConfirmation acknowledges an opt-out where one is owed.
	CategoryOptoutConfirmation Category = "optout_confirmation"
	// CategoryMarketing is advertising, read broadly.
	CategoryMarketing Category = "marketing"
)

// categories is the closed membership. A Category absent from this map is not
// a category, which is what Valid answers.
//
// Held by: TestNoGenericTransactionalCategoryExists (commsauthz_test.go), which
// fails if a generic operational member appears or the count drifts.
var categories = map[Category]bool{
	CategoryReplyToInbound: true, CategoryRequestedFollowup: true,
	CategoryPrecontractQuote: true, CategoryActiveDealFollowup: true,
	CategoryCustomerService: true, CategoryAccountNotice: true,
	CategoryContractNotice: true, CategoryInvoiceOrPayment: true,
	CategorySecurityNotice: true, CategoryPrivacyNotice: true,
	CategoryRecordConfirmation: true, CategoryConsentConfirmation: true,
	CategoryOptoutConfirmation: true, CategoryMarketing: true,
}

// Valid reports whether c is a member of the closed vocabulary.
func (c Category) Valid() bool { return categories[c] }

// Categories lists every category, for a test or a contract enum that must
// agree with this file rather than restate it.
func Categories() []Category {
	out := make([]Category, 0, len(categories))
	for c := range categories {
		out = append(out, c)
	}
	return out
}

// CarriesUnsubscribe reports whether a message of this category must offer the
// recipient a way to stop hearing from us.
//
// MARKETING ALONE, and the narrowness is the point rather than an omission. An
// unsubscribe surface is an offer to stop a stream the recipient can decline —
// and every other category is a message they cannot sensibly decline without
// declining the relationship itself: a reply to their own question, an invoice,
// a notice about their contract. Offering to unsubscribe from a conversation
// somebody started reads as not having read it.
//
// It replaces a rule keyed on the deprecated purpose key, which asked whether
// the purpose was the locked `transactional` one. That question had no answer
// for a send carrying no purpose key — and "not transactional" is not the same
// as "marketing", so a message with no key at all came out the far side wearing
// an unsubscribe footer.
//
// Held by: TestOnlyMarketingCarriesAnUnsubscribeSurface (backend/internal/modules/activities/unsubscribesurface_test.go)
func (c Category) CarriesUnsubscribe() bool { return c == CategoryMarketing }

// ServesTheSubject reports whether this category exists to serve the recipient
// rather than the sender. These five may pass a hard suppression, and only
// through a registered template: a security warning, a privacy notice, or the
// acknowledgement of somebody's own opt-out are messages a person is worse off
// for not receiving. A marketing objection does not reach them, because it is
// an objection to marketing.
//
// A hard bounce still stops all five. No template makes a dead address live.
func (c Category) ServesTheSubject() bool {
	switch c {
	case CategorySecurityNotice, CategoryPrivacyNotice, CategoryOptoutConfirmation,
		CategoryConsentConfirmation, CategoryRecordConfirmation:
		return true
	default:
		return false
	}
}
