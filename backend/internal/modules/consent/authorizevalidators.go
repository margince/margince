// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Whether the record bears out the category a message claims to be.
//
// One validator per category, each bound to tables that already exist. A
// category with no validator is UNSUPPORTED — never allowed by default — so
// adding a member to the vocabulary without teaching this file about it fails
// closed rather than open.
//
// WHAT A VALIDATOR MUST NOT DO is refuse ordinary correspondence. A wrong
// refusal is the failure mode that makes reps distrust the product and route
// around it, and it costs more than the permission it was protecting. So a
// validator that cannot find its evidence answers "review, and here is what to
// link" rather than "no" — the send parks under the shipped posture, and goes only where a category has been moved back to observe,
// and a human is told what the record is missing.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/contractterm"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// validate answers whether the record supports the claimed category for this
// recipient, and on what ground.
//
// It runs only for a claim that reached arm 3 of resolution: no thread and no
// live deal already answered, so this is the caller saying what the message is
// and the engine going to look.
func (g *Gate) validate(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, category commsauthz.Category, w packRules) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if subject.Kind != entityContact && category != commsauthz.CategoryReplyToInbound &&
		category != commsauthz.CategoryRequestedFollowup && category != commsauthz.CategoryPrivacyNotice {
		// A LEAD REACHES ONE ARM AND NO OTHER.
		//
		// The correspondence arm asks who wrote to us through subjectAskedUs.
		// wroteToUsWithin matches a lead through its bare address, and
		// requestOnRecord answers false for a lead without reading a
		// contact-keyed table.
		//
		// The privacy notice is the one link a lead can hold. A website lead
		// is told by it before anything else is sent.
		//
		// Every other arm reads a record a lead cannot hold. An invoice or
		// contract hangs off a company, and the record and consent links name
		// a contact. Those fall through to the lead's own grant.
		return unsupported, nil
	}
	// THE NAMED RECORD MUST BE ONE THE CALLER MAY SEE, before it is read.
	//
	// Evidence ids arrive on the request body and — unlike Links, which
	// SendOrigin.resolve puts through auth.EnsureLinkTarget before the consent
	// gate — nothing probes them upstream. Without this a seat with no finance
	// or contract grant could name any invoice id in the installation and have
	// the engine answer about it, which is both an unauthorized read and an
	// existence oracle for records they may not open.
	if err := refuseUnreadableEvidence(ctx, tx, req); err != nil {
		return resolution{}, err
	}
	res, err := g.validateCategory(ctx, tx, req, subject, category, w, unsupported)
	if err != nil {
		return resolution{}, err
	}
	// PAST THE READABILITY CHECK, so the ids this request named may be written
	// down for the transmit phase to ask about again.
	//
	// Stamped here rather than inside each arm because it is a fact about the
	// PATH and not about the answer: refuseUnreadableEvidence ran and admitted,
	// which is true of every arm below this line and of no arm above it.
	res.EvidenceChecked = true
	return res, nil
}

// validateCategory is validate's dispatch, split out so the readability check
// above and the EvidenceChecked stamp bracket every arm — rather than being
// repeated in six places and forgotten in the seventh somebody adds.
func (g *Gate) validateCategory(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, category commsauthz.Category, w packRules, unsupported resolution) (resolution, error) {
	switch category {
	case commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup:
		return g.validateRequestedFollowup(ctx, tx, subject, w, category)
	case commsauthz.CategoryActiveDealFollowup:
		return validateDealFollowup(ctx, tx, req, subject)
	case commsauthz.CategoryInvoiceOrPayment:
		return validateInvoice(ctx, tx, req, subject)
	case commsauthz.CategoryContractNotice:
		return validateContract(ctx, tx, req, subject)
	case commsauthz.CategoryPrecontractQuote:
		return validateQuote(ctx, tx, req, subject)
	case commsauthz.CategoryRecordConfirmation, commsauthz.CategoryConsentConfirmation,
		commsauthz.CategoryPrivacyNotice:
		return validateConfirmation(ctx, tx, subject, category)
	case commsauthz.CategoryCustomerService, commsauthz.CategoryAccountNotice, commsauthz.CategorySecurityNotice:
		return validateCustomerNotice(ctx, tx, subject, category)
	case commsauthz.CategoryOptoutConfirmation:
		// A DIFFERENT EVIDENCE from the three above, which is why it is its own
		// arm. Those carry a link and the live link is the evidence; an
		// acknowledgement carries nothing to click, so what it shows is the
		// standing stop it acknowledges.
		return validateOptOutAcknowledgement(ctx, tx, subject, category)
	default:
		// Every other category, marketing among them, has no record evidence
		// this file can read today. It stays unsupported and falls through to
		// the legacy verdict.
		return unsupported, nil
	}
}

// validateRequestedFollowup answers an unprompted follow-up through
// subjectAskedUs. A request a rep logged once is not asked for again on every
// later message.
func (g *Gate) validateRequestedFollowup(ctx context.Context, tx pgx.Tx, subject subjectRef, w packRules, category commsauthz.Category) (resolution, error) {
	found, err := subjectAskedUs(ctx, tx, subject, w)
	if err != nil {
		return resolution{}, err
	}
	if !found {
		return resolution{
			Category: category, Supported: false,
			Reason: commsauthz.ReasonNoEvidence,
		}, nil
	}
	return resolution{
		Category:  category,
		Basis:     commsauthz.BasisSubjectInitiatedCorrespondence,
		Supported: true,
	}, nil
}

// subjectAskedUs answers whether the record shows the subject asking to hear
// from us. Either they wrote or met with us inside the window, or a request of
// theirs is on record at any age.
func subjectAskedUs(ctx context.Context, tx pgx.Tx, subject subjectRef, w packRules) (bool, error) {
	found, err := repliedOrWasRecorded(ctx, tx, subject, time.Now(), w.reply)
	if err != nil || found {
		return found, err
	}
	return requestOnRecord(ctx, tx, subject, followupRequests)
}

// repliedOrWasRecorded asks whether the subject wrote to us, or somebody
// recorded that they asked to hear from us, inside the window.
//
// A contact is asked through latestQualifyingEvent, the reader the legacy
// correspondence verdict uses. So a recorded "requested by the subject" or an
// in-person exchange supports a composer send, and both readers agree. A lead
// has no qualifying event, because the table is keyed on contacts. It keeps
// the authorship test alone, which wroteToUsWithin spells for both kinds.
func repliedOrWasRecorded(ctx context.Context, tx pgx.Tx, subject subjectRef, asOf time.Time, window time.Duration) (bool, error) {
	since := asOf.Add(-window)
	if subject.Kind != entityContact {
		return wroteToUsWithin(ctx, tx, subject, since)
	}
	_, source, err := latestQualifyingEvent(ctx, tx, subject.ID, since, asOf, &subject.Address, subject.SendingEntity)
	if err != nil {
		return false, err
	}
	return source != sourceNone, nil
}

// validateDealFollowup answers a follow-up on a deal the sender picked. It reads
// the picked deal through liveDealInLinks and requestedNegotiation, so the
// links and the picked deal are asked one way. A live deal the contact never
// asked about gets its own reason: the deal is there, their request about it
// is not.
//
// A message filed under a deal names that deal even when no evidence field
// repeats it. The refusal then says what is missing, instead of asking for a
// deal the message already carries. The links were probed for visibility before
// the gate, and the outcome is unchanged: resolveCategory asked the same links.
func validateDealFollowup(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	deals := req.Links
	if req.Evidence.DealID != (ids.UUID{}) {
		deals = []ids.UUID{req.Evidence.DealID}
	}
	if len(deals) == 0 {
		return resolution{Category: commsauthz.CategoryActiveDealFollowup, Reason: commsauthz.ReasonNoEvidence}, nil
	}
	live, err := liveDealInLinks(ctx, tx, deals, subject)
	if err != nil {
		return resolution{}, err
	}
	if !live {
		return resolution{Category: commsauthz.CategoryActiveDealFollowup, Reason: commsauthz.ReasonNoEvidence}, nil
	}
	asked, err := requestedNegotiation(ctx, tx, deals, subject)
	if err != nil {
		return resolution{}, err
	}
	if !asked {
		return resolution{Category: commsauthz.CategoryActiveDealFollowup, Reason: commsauthz.ReasonDealNotRequested}, nil
	}
	return resolution{
		Category:  commsauthz.CategoryActiveDealFollowup,
		Basis:     commsauthz.BasisLegitimateInterests,
		Supported: true,
	}, nil
}

// validateInvoice answers a message about a named financial event.
//
// An invoice belongs to a company, and its mail goes to the company's billing
// contact (documentRecipientSQL). Employment alone chooses nobody. A recipient
// the record does not name is unsupported, never refused. The operator is told
// what to link.
func validateInvoice(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateCompanyDocument(ctx, tx, subject, commsauthz.CategoryInvoiceOrPayment,
		commsauthz.BasisContract, `
		SELECT EXISTS (
			SELECT 1 FROM finance_invoice i
			 WHERE i.id = $1::uuid
			   -- A deleted or voided invoice is not a financial event anybody
			   -- is owed a message about.
			   AND i.archived_at IS NULL
			   AND i.void_at IS NULL
			   AND `+documentRecipientSQL("i.company_id", "$3::uuid")+`
		)`, req.Evidence.InvoiceID, zeroAsNull(req.Evidence.DealID), buyerRoles)
}

// validateContract answers a notice a contract in force requires. Same
// recipient rule as the invoice. The deal is the contract's own, else the one named.
func validateContract(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateCompanyDocument(ctx, tx, subject, commsauthz.CategoryContractNotice,
		commsauthz.BasisContract, `
		SELECT EXISTS (
			SELECT 1 FROM contract c
			 WHERE c.id = $1::uuid
			   AND `+contractCarriesMailSQL("c")+`
			   AND `+documentRecipientSQL("c.company_id", "coalesce(c.deal_id, $3::uuid)")+`
		)`, req.Evidence.ContractID, zeroAsNull(req.Evidence.DealID), buyerRoles)
}

// contractCarriesMailSQL is true when the contract aliased alias supports mail
// today. It must be in force (contractterm.InForceSQL). Its status must agree:
// active, or cancelled with the cancellation taking effect after today. The contracts module judges by dates alone, so an expired
// or cancelled contract with no dates still counts there; here it does not.
func contractCarriesMailSQL(alias string) string {
	return "(" + contractterm.InForceSQL(alias, "current_date") +
		" AND (" + alias + ".status = 'active' OR (" + alias + ".status = 'cancelled' AND " +
		alias + ".cancellation_effective_on > current_date)))"
}

// documentRecipientSQL is true when the recipient in $2 is who an invoice or a
// contract of the company goes to: a billing contact of the company. A company
// with none sends to the buyer on the deal (buyerRoles, in $4), and only on a
// deal of that company.
func documentRecipientSQL(company, deal string) string {
	return `(EXISTS (SELECT 1 FROM relationship b
		              WHERE b.kind = 'billing_contact' AND b.company_id = ` + company + `
		                AND b.contact_id = $2::uuid AND b.archived_at IS NULL)
		  OR (NOT EXISTS (SELECT 1 FROM relationship b
		                   WHERE b.kind = 'billing_contact' AND b.company_id = ` + company + `
		                     AND b.archived_at IS NULL)
		      AND EXISTS (SELECT 1 FROM relationship r JOIN deal d ON d.id = r.deal_id
		                   WHERE r.kind = 'deal_stakeholder' AND r.deal_id = ` + deal + `
		                     AND d.company_id = ` + company + ` AND d.archived_at IS NULL
		                     AND r.contact_id = $2::uuid AND r.role = ANY($4)
		                     AND r.ended_at IS NULL AND r.archived_at IS NULL)))`
}

// validateCustomerNotice answers service, account and security mail to a
// customer. A customer is a contact currently employed at a company with a
// contract in force or a won deal. No inline fact is needed; the record carries the relationship.
func validateCustomerNotice(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category) (resolution, error) {
	var customer bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM relationship r
			 WHERE r.kind = 'employment' AND r.contact_id = $1::uuid
			   AND `+employment.IsCurrentSQL("r.ended_at")+`
			   AND r.archived_at IS NULL
			   AND (EXISTS (SELECT 1 FROM contract c
			                 WHERE c.company_id = r.company_id
			                   AND `+contractCarriesMailSQL("c")+`)
			        OR EXISTS (SELECT 1 FROM deal d
			                    WHERE d.company_id = r.company_id AND d.archived_at IS NULL
			                      AND d.status = 'won')))`, subject.ID).Scan(&customer); err != nil {
		return resolution{}, wrapEvidenceRead("whether the recipient works for a customer", err)
	}
	if !customer {
		return resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}, nil
	}
	basis := commsauthz.BasisContract
	if category == commsauthz.CategorySecurityNotice {
		basis = commsauthz.BasisVitalOrSecurityInterest
	}
	return resolution{Category: category, Basis: basis, Supported: true}, nil
}

// validateQuote answers a requested quote or offer. Either a sent offer on the
// named deal reaches this contact, or a request for a quote is on record. The
// second is the quote written from the enquiry before anyone made a deal or an
// offer, and it needs no newsletter consent. Correspondence alone is not a
// request for a quote; a reply in the enquiry's own thread is a reply.
//
// The request arm is the weaker ground, so a marketing objection recorded
// after the request binds it (MarketingStopsBind). Only a request about this
// quote's deal, or naming no deal, is read for it.
func validateQuote(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	res, err := quoteOnDeal(ctx, tx, req, subject)
	if err != nil || res.Supported {
		return res, err
	}
	askedAt, asked, err := latestRequest(ctx, tx, subject, quoteRequests, req.Evidence.DealID)
	if err != nil || !asked {
		return res, err
	}
	return resolution{
		Category:           commsauthz.CategoryPrecontractQuote,
		Basis:              commsauthz.BasisPrecontractRequest,
		Supported:          true,
		MarketingStopsBind: true,
		RequestedAt:        askedAt,
	}, nil
}

// quoteOnDeal answers a quote on a named deal: a sent offer on a live deal
// whose stakeholder is this contact. Being on the opportunity is what makes
// somebody the contact a quote goes to.
func quoteOnDeal(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateCompanyDocument(ctx, tx, subject, commsauthz.CategoryPrecontractQuote,
		commsauthz.BasisPrecontractRequest, `
		SELECT EXISTS (
			SELECT 1 FROM offer o
			  JOIN deal d ON d.id = o.deal_id
			  JOIN relationship r ON r.deal_id = o.deal_id
			 WHERE o.deal_id = $1::uuid
			   AND o.archived_at IS NULL
			   -- A quote that was REACHED the buyer. A draft was never sent, and
			   -- a rejected, expired or superseded one is a conversation that
			   -- ended — none of the three is a precontract request anybody is
			   -- still waiting on.
			   AND o.status IN ('sent', 'accepted')
			   -- And the opportunity has to be live, the same test
			   -- liveDealInLinks applies: a closed-lost deal from three years
			   -- ago is not a quote in progress.
			   AND d.status = 'open'
			   AND d.archived_at IS NULL
			   AND r.kind = 'deal_stakeholder'
			   AND r.contact_id = $2::uuid
			   AND r.ended_at IS NULL
			   AND r.archived_at IS NULL
		)`, req.Evidence.DealID)
}

// wrapEvidenceRead names the read that failed without naming the record, the
// recipient or the company it was about. A decision's errors reach an
// operator's lane.
func wrapEvidenceRead(what string, err error) error {
	return fmt.Errorf("consent: read %s: %w", what, err)
}

// validateCompanyDocument runs the shared shape of the three document validators:
// the caller named a record, and the recipient is reachable from it.
//
// One function because the three differ only in their query and their basis. A
// second copy of "named nothing, so unsupported" is a second place for the
// no-evidence answer to drift.
//
// args are the query's parameters after the named record ($1) and the
// recipient ($2).
func validateCompanyDocument(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category, basis commsauthz.Basis, query string, named ids.UUID, args ...any) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if named == (ids.UUID{}) {
		// The caller claimed the category and named no record. Nothing to look
		// at, so nothing supports it.
		return unsupported, nil
	}
	var found bool
	if err := tx.QueryRow(ctx, query, append([]any{named, subject.ID}, args...)...).Scan(&found); err != nil {
		return resolution{}, wrapEvidenceRead("the record this message is about", err)
	}
	if !found {
		return unsupported, nil
	}
	return resolution{Category: category, Basis: basis, Supported: true}, nil
}

// refuseUnreadableEvidence checks the caller may see every record they named.
//
// Refused as NOT FOUND rather than forbidden, matching auth.EnsureVisible: a
// caller who may not read a record must not learn it exists, and a 403 on a
// guessed id answers exactly that question.
//
// Two shapes, because the tree gates these tables two ways. A deal is
// row-scoped, so the probe is the same one the link path uses. An invoice and a
// contract are governed by object grants — neither is in auth's row-scoped set
// — so the check is the grant their own read handlers ask for.
func refuseUnreadableEvidence(ctx context.Context, tx pgx.Tx, req commsauthz.Request) error {
	if req.Evidence.DealID != (ids.UUID{}) {
		if err := auth.EnsureLinkTarget(ctx, tx, "deal", req.Evidence.DealID); err != nil {
			return err
		}
	}
	for _, named := range []struct {
		id     ids.UUID
		object string
	}{
		{req.Evidence.InvoiceID, "finance"},
		{req.Evidence.ContractID, "contract"},
	} {
		if named.id == (ids.UUID{}) {
			continue
		}
		if err := auth.Require(ctx, named.object, principal.ActionRead); err != nil {
			// The grant refusal becomes a not-found, so naming an id the
			// caller may not read discloses nothing about whether it exists.
			return fmt.Errorf("consent: this message names a record that is not available: %w",
				apperrors.ErrNotFound)
		}
	}
	return nil
}
