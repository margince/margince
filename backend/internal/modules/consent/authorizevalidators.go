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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	switch category {
	case commsauthz.CategoryReplyToInbound, commsauthz.CategoryRequestedFollowup:
		return g.validateRequestedFollowup(ctx, tx, subject, w, category)
	case commsauthz.CategoryInvoiceOrPayment:
		return validateInvoice(ctx, tx, req, subject)
	case commsauthz.CategoryContractNotice:
		return validateContract(ctx, tx, req, subject)
	case commsauthz.CategoryPrecontractQuote:
		return validateQuote(ctx, tx, req, subject)
	default:
		// Every other category — the five that serve the subject, marketing,
		// customer service, account notices — has no record evidence this file
		// can read today. They stay unsupported and fall through to the legacy
		// verdict, which is exactly what they did before this file existed.
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
	var found bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM activity a
			  JOIN activity_link l ON l.activity_id = a.id AND l.person_id = $1::uuid
			 WHERE a.direction = 'inbound'
			   AND a.archived_at IS NULL
			   AND a.occurred_at >= $2
		) OR EXISTS (
			SELECT 1 FROM person_acquisition_evidence e
			 WHERE e.person_id = $1::uuid
			   -- The two kinds that ARE a request to be contacted. The others
			   -- record where a contact came from, which is provenance and
			   -- never permission: a purchased list and a public source say
			   -- nothing about what this person asked for.
			   AND e.kind IN ('requested_quote_or_meeting', 'in_person_permission')
			   AND coalesce(e.occurred_at, e.captured_at) >= $2
		)`, subject.ID, time.Now().Add(-w.reply)).Scan(&found); err != nil {
		return resolution{}, wrapEvidenceRead("the follow-up this person asked for", err)
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
			  JOIN relationship r ON r.deal_id = o.deal_id
			 WHERE o.deal_id = $1::uuid
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
