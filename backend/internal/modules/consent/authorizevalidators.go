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
// link" rather than "no" — the send still goes while the engine is observed,
// and a human is told what the record is missing.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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
func (g *Gate) validate(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef, category commsauthz.Category, w windows) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if subject.Kind != entityPerson {
		// Only the person arm has record evidence to read: invoices and
		// contracts hang off an organization reached through employment, and a
		// lead holds none of those. A lead's own answers come from the legacy
		// verdict path, which decideLead reaches without passing through here.
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
	switch category {
	case commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup:
		return g.validateRequestedFollowup(ctx, tx, subject, w, category)
	case commsauthz.CategoryInvoiceOrPayment:
		return validateInvoice(ctx, tx, req, subject)
	case commsauthz.CategoryContractNotice:
		return validateContract(ctx, tx, req, subject)
	case commsauthz.CategoryPrecontractQuote:
		return validateQuote(ctx, tx, req, subject)
	case commsauthz.CategoryRecordConfirmation, commsauthz.CategoryConsentConfirmation:
		return validateConfirmation(ctx, tx, subject, category)
	default:
		// Every other category — marketing, customer service, account notices,
		// and the three subject-serving ones that carry no link — has no record
		// evidence this file can read today. They stay unsupported and fall
		// through to the legacy verdict, which is exactly what they did before
		// this file existed.
		return unsupported, nil
	}
}

// validateRequestedFollowup answers an unprompted follow-up: is there something
// on file, inside the window, that says this person asked to hear from us.
//
// TWO SOURCES, and the second is why a first mail to somebody who phoned is not
// refused. An inbound message is the obvious one. The other is the acquisition
// evidence a contact was created with (person_acquisition_evidence, written by
// every creation door): a rep who logged "they asked me for a quote at the
// trade fair" has recorded the request, and asking them to record it a second
// time in a different table would be asking them to restate what the CRM
// already knows.
func (g *Gate) validateRequestedFollowup(ctx context.Context, tx pgx.Tx, subject subjectRef, w windows, category commsauthz.Category) (resolution, error) {
	// AUTHORSHIP, through the shared reader. An earlier version asked
	// activity_link — a FILING link with no author concept — which read "some
	// inbound activity is filed under this person". A caller may post an
	// activity with direction=inbound and a link to any contact they can read,
	// so that let anybody manufacture their own evidence.
	found, err := wroteToUsWithin(ctx, tx, subject, time.Now().Add(-w.reply))
	if err != nil {
		return resolution{}, err
	}
	if !found {
		asked, err := askedToBeContacted(ctx, tx, subject, time.Now().Add(-w.reply))
		if err != nil {
			return resolution{}, err
		}
		found = asked
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

// validateInvoice answers a message about a named financial event.
//
// An invoice belongs to an ORGANIZATION, so reaching a person means going
// through employment. That is a real gap in ordinary CRM data — a finance
// contact who was never linked to the customer record — and it is a data gap
// rather than a legal one. So a missing link is unsupported with a reason a
// human can act on, never a refusal: the legacy path still answers, and the
// operator is told what to link.
func validateInvoice(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateOrgDocument(ctx, tx, subject, commsauthz.CategoryInvoiceOrPayment,
		commsauthz.BasisContract, `
		SELECT EXISTS (
			SELECT 1 FROM finance_invoice i
			  JOIN relationship r ON r.organization_id = i.organization_id
			 WHERE i.id = $1::uuid
			   -- A deleted or voided invoice is not a financial event anybody
			   -- is owed a message about. The contract validator beside this
			   -- already excludes its archived rows; this one did not, so a
			   -- voided invoice authorized a send forever.
			   AND i.archived_at IS NULL
			   AND i.void_at IS NULL
			   AND r.kind = 'employment'
			   AND r.person_id = $2::uuid
			   -- A DATE comparison, not a null check: somebody serving three
			   -- months' notice still works there, and reading the column's
			   -- presence as "gone" would take them off their employer's
			   -- contact list the day their notice was filed. This is
			   -- people.EmploymentIsCurrentSQL spelled out — consent may not
			   -- import a sibling module (ADR-0054 §3), so it is ratified by
			   -- name in the employment-currency gate alongside the five other
			   -- statements in the same position.
			   AND (r.ended_at IS NULL OR r.ended_at > current_date)
			   AND r.archived_at IS NULL
		)`, req.Evidence.InvoiceID)
}

// validateContract answers a notice a live contract requires. Same shape and
// same reasoning as the invoice: the document names an organization, and the
// person is reached through employment.
func validateContract(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateOrgDocument(ctx, tx, subject, commsauthz.CategoryContractNotice,
		commsauthz.BasisContract, `
		SELECT EXISTS (
			SELECT 1 FROM contract c
			  JOIN relationship r ON r.organization_id = c.organization_id
			 WHERE c.id = $1::uuid
			   AND c.archived_at IS NULL
			   AND r.kind = 'employment'
			   AND r.person_id = $2::uuid
			   -- A DATE comparison, not a null check: somebody serving three
			   -- months' notice still works there, and reading the column's
			   -- presence as "gone" would take them off their employer's
			   -- contact list the day their notice was filed. This is
			   -- people.EmploymentIsCurrentSQL spelled out — consent may not
			   -- import a sibling module (ADR-0054 §3), so it is ratified by
			   -- name in the employment-currency gate alongside the five other
			   -- statements in the same position.
			   AND (r.ended_at IS NULL OR r.ended_at > current_date)
			   AND r.archived_at IS NULL
		)`, req.Evidence.ContractID)
}

// validateQuote answers a requested quote or offer. The offer hangs off a deal,
// and the person is reached as a stakeholder on that deal — the same edge the
// follow-up arm reads, because being on the opportunity is what makes somebody
// the person a quote goes to.
func validateQuote(ctx context.Context, tx pgx.Tx, req commsauthz.Request, subject subjectRef) (resolution, error) {
	return validateOrgDocument(ctx, tx, subject, commsauthz.CategoryPrecontractQuote,
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
			   AND r.person_id = $2::uuid
			   AND r.ended_at IS NULL
			   AND r.archived_at IS NULL
		)`, req.Evidence.DealID)
}

// wrapEvidenceRead names the read that failed without naming the record, the
// recipient or the organization it was about. A decision's errors reach an
// operator's lane.
func wrapEvidenceRead(what string, err error) error {
	return fmt.Errorf("consent: read %s: %w", what, err)
}

// validateOrgDocument runs the shared shape of the three document validators:
// the caller named a record, and the recipient is reachable from it.
//
// One function because the three differ only in their query and their basis. A
// second copy of "named nothing, so unsupported" is a second place for the
// no-evidence answer to drift.
func validateOrgDocument(ctx context.Context, tx pgx.Tx, subject subjectRef, category commsauthz.Category, basis commsauthz.Basis, query string, named ids.UUID) (resolution, error) {
	unsupported := resolution{Category: category, Supported: false, Reason: commsauthz.ReasonNoEvidence}
	if named == (ids.UUID{}) {
		// The caller claimed the category and named no record. Nothing to look
		// at, so nothing supports it.
		return unsupported, nil
	}
	var found bool
	if err := tx.QueryRow(ctx, query, named, subject.ID).Scan(&found); err != nil {
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
