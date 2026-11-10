// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The sentences this product writes onto an approval for a human to decide.
//
// approval.summary is shared-record text: stored once, read by every seat that
// can see the card. So it follows the installation's base language, resolved
// when the card is staged, exactly as a signal summary does (signalsummarycopy.go)
// and for the same reason — a sentence in one hardcoded language reads correctly
// only on the installations that happened to choose it. A card already stored
// keeps the words it was written in.
//
// Only product-written copy lives here. A summary a model wrote is governed by
// promptlang.Rule; a subject line, a headline, a name or a URL is quoted from
// its source and passes through every language untouched.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// approvalSummaryCopy is one language's set. Every string field is required,
// and the census walks the struct itself, so a field added here is covered
// without anybody remembering to list it.
type approvalSummaryCopy struct {
	taskCompleted    string
	taskContact      string
	signaturePrivate string
	// draftedReplyWaiting: a follow-up reply is drafted. %q is its subject.
	draftedReplyWaiting string
	// counterpartyWorthKeeping asks about an unresolved sender. %s is the address.
	counterpartyWorthKeeping string
	// contactShareAsked: %s is the colleague asking for the contact.
	contactShareAsked string
	// companyRename: %s is the current name, then the proposed one.
	companyRename string
	// siteLeadFound: %s is the site's host, then the contact's name, then role.
	siteLeadFound string
	// enrichmentOf: %s is the URL the enrichment reads.
	enrichmentOf string
	// contractEndedMove: %s is the account's current stage, then the proposed one.
	contractEndedMove string
	// linkedInLooksLike: %s is the connection, their employer, then the contact.
	linkedInLooksLike string
	// vcardResemblesContact: %s is the card's name, after the qualifier.
	vcardResemblesContact string
	// vcardReadFromPhoto: %s is the card's name, after the qualifier.
	vcardReadFromPhoto string
	// unnamedEmployer stands in for an employer the export left blank, inside
	// linkedInLooksLike's second hole.
	unnamedEmployer string

	// heldNotSent: %q is the message's subject, %s one of the held* reasons.
	heldNotSent          string
	heldConsentWithdrawn string
	heldSenderInactive   string
	heldPassportRevoked  string
	heldMissedWindow     string
	heldTimerExhausted   string
	heldSendRefused      string
	heldFollowupSettled  string
	heldMeetingCanceled  string
	heldRecipientStopped string
	heldRecipientReplied string

	// reviewSendRefused: %s is reviewOneRecipient or reviewRecipients, then the
	// refusal's reason code, which is an identifier and stays untranslated.
	reviewSendRefused  string
	reviewOneRecipient string
	reviewRecipients   string

	// The cold-start read-back names its source; %s is the URL.
	coldStartFromURL             string
	coldStartFromText            string
	coldStartFromSelfDescription string

	// fxRateChanged: %s is the currency, the base, the new rate, then the prior
	// rate or fxNoRateInForce.
	fxRateChanged   string
	fxNoRateInForce string

	// The REST gate's structural summary: the pseudo-field naming a nested
	// create's parent, and the count that stands in for a nested object. The
	// count of fields left out is the tool door's (agents.MoreFields).
	createdUnder string
	nestedFields string

	// acts names what an agent's staged call does (approvalsummarycopyagent.go).
	acts agentActVocabulary
	// lang is the set's own language, stamped by approvalSummaryCopyFor for the
	// words this set borrows from the tool door.
	lang textlang.Lang
}

// approvalSummaryByLang is the census, keyed by textlang.Lang so the test can
// walk textlang.Shipped and ask this map directly.
var approvalSummaryByLang = map[textlang.Lang]approvalSummaryCopy{
	textlang.English: {
		taskCompleted:            systemtext.Copy("approval.taskCompleted", textlang.English),
		taskContact:              systemtext.Copy("approval.taskContact", textlang.English),
		signaturePrivate:         systemtext.Copy("approval.signaturePrivate", textlang.English),
		draftedReplyWaiting:      systemtext.Copy("approval.draftedReplyWaiting", textlang.English),
		counterpartyWorthKeeping: systemtext.Copy("approval.counterpartyWorthKeeping", textlang.English),
		contactShareAsked:        systemtext.Copy("approval.contactShareAsked", textlang.English),
		companyRename:            systemtext.Copy("approval.companyRename", textlang.English),
		siteLeadFound:            systemtext.Copy("approval.siteLeadFound", textlang.English),
		enrichmentOf:             systemtext.Copy("approval.enrichmentOf", textlang.English),
		contractEndedMove:        systemtext.Copy("approval.contractEndedMove", textlang.English),
		linkedInLooksLike:        systemtext.Copy("approval.linkedInLooksLike", textlang.English),
		unnamedEmployer:          systemtext.Copy("approval.unnamedEmployer", textlang.English),
		vcardResemblesContact:    systemtext.Copy("approval.vcardResemblesContact", textlang.English),
		vcardReadFromPhoto:       systemtext.Copy("approval.vcardReadFromPhoto", textlang.English),

		heldNotSent:          systemtext.Copy("approval.heldNotSent", textlang.English),
		heldConsentWithdrawn: systemtext.Copy("approval.heldConsentWithdrawn", textlang.English),
		heldSenderInactive:   systemtext.Copy("approval.heldSenderInactive", textlang.English),
		heldPassportRevoked:  systemtext.Copy("approval.heldPassportRevoked", textlang.English),
		heldMissedWindow:     systemtext.Copy("approval.heldMissedWindow", textlang.English),
		heldTimerExhausted:   systemtext.Copy("approval.heldTimerExhausted", textlang.English),
		heldSendRefused:      systemtext.Copy("approval.heldSendRefused", textlang.English),
		heldFollowupSettled:  systemtext.Copy("approval.heldFollowupSettled", textlang.English),
		heldMeetingCanceled:  systemtext.Copy("approval.heldMeetingCanceled", textlang.English),
		heldRecipientStopped: systemtext.Copy("approval.heldRecipientStopped", textlang.English),
		heldRecipientReplied: systemtext.Copy("approval.heldRecipientReplied", textlang.English),

		reviewSendRefused:  systemtext.Copy("approval.reviewSendRefused", textlang.English),
		reviewOneRecipient: systemtext.Copy("approval.reviewOneRecipient", textlang.English),
		reviewRecipients:   systemtext.Copy("approval.reviewRecipients", textlang.English),

		coldStartFromURL:             systemtext.Copy("approval.coldStartFromURL", textlang.English),
		coldStartFromText:            systemtext.Copy("approval.coldStartFromText", textlang.English),
		coldStartFromSelfDescription: systemtext.Copy("approval.coldStartFromSelfDescription", textlang.English),

		fxRateChanged:   systemtext.Copy("approval.fxRateChanged", textlang.English),
		fxNoRateInForce: systemtext.Copy("approval.fxNoRateInForce", textlang.English),

		createdUnder: systemtext.Copy("approval.createdUnder", textlang.English),
		nestedFields: systemtext.Copy("approval.nestedFields", textlang.English),

		acts: agentActsEnglish,
	},
	textlang.German: {
		taskCompleted:            systemtext.Copy("approval.taskCompleted", textlang.German),
		taskContact:              systemtext.Copy("approval.taskContact", textlang.German),
		signaturePrivate:         systemtext.Copy("approval.signaturePrivate", textlang.German),
		draftedReplyWaiting:      systemtext.Copy("approval.draftedReplyWaiting", textlang.German),
		counterpartyWorthKeeping: systemtext.Copy("approval.counterpartyWorthKeeping", textlang.German),
		contactShareAsked:        systemtext.Copy("approval.contactShareAsked", textlang.German),
		companyRename:            systemtext.Copy("approval.companyRename", textlang.German),
		siteLeadFound:            systemtext.Copy("approval.siteLeadFound", textlang.German),
		enrichmentOf:             systemtext.Copy("approval.enrichmentOf", textlang.German),
		contractEndedMove:        systemtext.Copy("approval.contractEndedMove", textlang.German),
		linkedInLooksLike:        systemtext.Copy("approval.linkedInLooksLike", textlang.German),
		unnamedEmployer:          systemtext.Copy("approval.unnamedEmployer", textlang.German),
		vcardResemblesContact:    systemtext.Copy("approval.vcardResemblesContact", textlang.German),
		vcardReadFromPhoto:       systemtext.Copy("approval.vcardReadFromPhoto", textlang.German),

		heldNotSent:          systemtext.Copy("approval.heldNotSent", textlang.German),
		heldConsentWithdrawn: systemtext.Copy("approval.heldConsentWithdrawn", textlang.German),
		heldSenderInactive:   systemtext.Copy("approval.heldSenderInactive", textlang.German),
		heldPassportRevoked:  systemtext.Copy("approval.heldPassportRevoked", textlang.German),
		heldMissedWindow:     systemtext.Copy("approval.heldMissedWindow", textlang.German),
		heldTimerExhausted:   systemtext.Copy("approval.heldTimerExhausted", textlang.German),
		heldSendRefused:      systemtext.Copy("approval.heldSendRefused", textlang.German),
		heldFollowupSettled:  systemtext.Copy("approval.heldFollowupSettled", textlang.German),
		heldMeetingCanceled:  systemtext.Copy("approval.heldMeetingCanceled", textlang.German),
		heldRecipientStopped: systemtext.Copy("approval.heldRecipientStopped", textlang.German),
		heldRecipientReplied: systemtext.Copy("approval.heldRecipientReplied", textlang.German),

		reviewSendRefused:  systemtext.Copy("approval.reviewSendRefused", textlang.German),
		reviewOneRecipient: systemtext.Copy("approval.reviewOneRecipient", textlang.German),
		reviewRecipients:   systemtext.Copy("approval.reviewRecipients", textlang.German),

		coldStartFromURL:             systemtext.Copy("approval.coldStartFromURL", textlang.German),
		coldStartFromText:            systemtext.Copy("approval.coldStartFromText", textlang.German),
		coldStartFromSelfDescription: systemtext.Copy("approval.coldStartFromSelfDescription", textlang.German),

		fxRateChanged:   systemtext.Copy("approval.fxRateChanged", textlang.German),
		fxNoRateInForce: systemtext.Copy("approval.fxNoRateInForce", textlang.German),

		createdUnder: systemtext.Copy("approval.createdUnder", textlang.German),
		nestedFields: systemtext.Copy("approval.nestedFields", textlang.German),

		acts: agentActsGerman,
	},
	textlang.Vietnamese: {
		taskCompleted:            systemtext.Copy("approval.taskCompleted", textlang.Vietnamese),
		taskContact:              systemtext.Copy("approval.taskContact", textlang.Vietnamese),
		signaturePrivate:         systemtext.Copy("approval.signaturePrivate", textlang.Vietnamese),
		draftedReplyWaiting:      systemtext.Copy("approval.draftedReplyWaiting", textlang.Vietnamese),
		counterpartyWorthKeeping: systemtext.Copy("approval.counterpartyWorthKeeping", textlang.Vietnamese),
		contactShareAsked:        systemtext.Copy("approval.contactShareAsked", textlang.Vietnamese),
		companyRename:            systemtext.Copy("approval.companyRename", textlang.Vietnamese),
		siteLeadFound:            systemtext.Copy("approval.siteLeadFound", textlang.Vietnamese),
		enrichmentOf:             systemtext.Copy("approval.enrichmentOf", textlang.Vietnamese),
		contractEndedMove:        systemtext.Copy("approval.contractEndedMove", textlang.Vietnamese),
		linkedInLooksLike:        systemtext.Copy("approval.linkedInLooksLike", textlang.Vietnamese),
		unnamedEmployer:          systemtext.Copy("approval.unnamedEmployer", textlang.Vietnamese),
		vcardResemblesContact:    systemtext.Copy("approval.vcardResemblesContact", textlang.Vietnamese),
		vcardReadFromPhoto:       systemtext.Copy("approval.vcardReadFromPhoto", textlang.Vietnamese),

		heldNotSent:          systemtext.Copy("approval.heldNotSent", textlang.Vietnamese),
		heldConsentWithdrawn: systemtext.Copy("approval.heldConsentWithdrawn", textlang.Vietnamese),
		heldSenderInactive:   systemtext.Copy("approval.heldSenderInactive", textlang.Vietnamese),
		heldPassportRevoked:  systemtext.Copy("approval.heldPassportRevoked", textlang.Vietnamese),
		heldMissedWindow:     systemtext.Copy("approval.heldMissedWindow", textlang.Vietnamese),
		heldTimerExhausted:   systemtext.Copy("approval.heldTimerExhausted", textlang.Vietnamese),
		heldSendRefused:      systemtext.Copy("approval.heldSendRefused", textlang.Vietnamese),
		heldFollowupSettled:  systemtext.Copy("approval.heldFollowupSettled", textlang.Vietnamese),
		heldMeetingCanceled:  systemtext.Copy("approval.heldMeetingCanceled", textlang.Vietnamese),
		heldRecipientStopped: systemtext.Copy("approval.heldRecipientStopped", textlang.Vietnamese),
		heldRecipientReplied: systemtext.Copy("approval.heldRecipientReplied", textlang.Vietnamese),

		reviewSendRefused:  systemtext.Copy("approval.reviewSendRefused", textlang.Vietnamese),
		reviewOneRecipient: systemtext.Copy("approval.reviewOneRecipient", textlang.Vietnamese),
		reviewRecipients:   systemtext.Copy("approval.reviewRecipients", textlang.Vietnamese),

		coldStartFromURL:             systemtext.Copy("approval.coldStartFromURL", textlang.Vietnamese),
		coldStartFromText:            systemtext.Copy("approval.coldStartFromText", textlang.Vietnamese),
		coldStartFromSelfDescription: systemtext.Copy("approval.coldStartFromSelfDescription", textlang.Vietnamese),

		fxRateChanged:   systemtext.Copy("approval.fxRateChanged", textlang.Vietnamese),
		fxNoRateInForce: systemtext.Copy("approval.fxNoRateInForce", textlang.Vietnamese),

		createdUnder: systemtext.Copy("approval.createdUnder", textlang.Vietnamese),
		nestedFields: systemtext.Copy("approval.nestedFields", textlang.Vietnamese),

		acts: agentActsVietnamese,
	},
}

// approvalSummaryCopyFor answers the installation's language, and English for
// anything else — the language comes off a settings row an admin can edit, and
// answering in a language beats answering in none.
func approvalSummaryCopyFor(lang textlang.Lang) approvalSummaryCopy {
	said, ok := approvalSummaryByLang[lang]
	if !ok {
		said, lang = approvalSummaryByLang[textlang.English], textlang.English
	}
	said.lang = lang
	return said
}

// approvalSummaryCopyIn is the set for a stager already inside a transaction.
func approvalSummaryCopyIn(ctx context.Context, tx pgx.Tx) approvalSummaryCopy {
	return approvalSummaryCopyFor(identity.BaseLanguageForRecord(ctx, tx))
}

// approvalSummaryCopyOver is the set for a stager holding only the pool.
func approvalSummaryCopyOver(ctx context.Context, pool *pgxpool.Pool) approvalSummaryCopy {
	return approvalSummaryCopyFor(installationLanguage(pool).Resolve(ctx))
}
