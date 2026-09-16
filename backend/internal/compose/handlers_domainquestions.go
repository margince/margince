// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Answering an open domain question: keep the company, or stop capturing the
// domain for the colleague who asked.
//
// Two verbs, two different stores, and that is the shape of the answer rather
// than an accident of wiring. KEEP is a company decision — it settles the
// triage ledger and mints the record the crawl withheld. DISCARD is a capture
// boundary — it writes the caller's OWN exclusion rule, which binds the
// connections they granted and nobody else's. Two colleagues on one
// installation may answer the same domain opposite ways and both be honoured,
// because the discard half was never workspace-wide.
//
// Thin transport, like the blocked-domain surface beside it: the stores own
// their gates, their normalization and their audited writes.

import (
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type domainQuestionHandlers struct {
	contacts   *contacts.Store
	exclusions *capture.ExclusionStore
}

// KeepDomainQuestion settles an undecided domain as a company.
//
// The company is named from the domain's own registrable label: nothing on the
// site named it, and the human pressing this is not asked to type one.
// ResolveDomainTriage falls back to that label when no dossier name is given,
// which is the same name the pre-triage path always produced — worse than a
// site would have given, never invented.
func (h domainQuestionHandlers) KeepDomainQuestion(w http.ResponseWriter, r *http.Request, domain string) {
	// Human-only (x-agent-access): capture posture, not record data.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The NORMALIZED form is what travels on. Validating and then passing the
	// raw string looks the same and is not: the disposition is keyed on the
	// registrable domain, so `EXAMPLE.COM` would miss the row it was asked
	// about and fail as an internal error rather than settling anything.
	base, ok := freemail.Hostname(domain)
	if !ok {
		httperr.Write(w, r, httperr.Validation("domain", "invalid",
			"expected a domain name like example.com; a full email address or a URL is not one"))
		return
	}
	res, err := h.contacts.ResolveDomainTriage(r.Context(), contacts.ResolveDomainTriageInput{
		Domain: base,
		Status: contacts.DomainCompany,
		// HUMAN, not heuristic: a colleague looked at the domain and said it is
		// a company. The triage reads this too — a human answer is not withheld
		// for stale evidence, because a human is exactly who that withholding
		// was waiting for.
		Source: contacts.DomainSourceHuman,
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if res.CompanyID == nil {
		// The resolve REFUSED without erroring. Two states answer this way and
		// the message must not claim one of them: a suppressed domain (a crawl
		// in flight must not create what a suppression forbids) and a domain
		// already settled as something other than a company. Both mean the
		// row's state refuses, which is a conflict rather than a fault.
		httperr.Write(w, r, fmt.Errorf(
			"compose: %s already carries an answer that keeping it would overturn: %w",
			base, apperrors.ErrConflict))
		return
	}
	// ADMITTED, recorded as a human decision. Two things hang on this beyond
	// the response: the sticky rule in setDomainAdmissionTx guards on
	// admission_source, so without it the next automatic bulk-sender verdict
	// may suppress the domain a colleague just kept; and settleDisposition
	// writes none of the admission columns, so the row would answer with an
	// empty admission and an empty source — neither of which the contract
	// publishes. It carries the company-update gate the contract promises for
	// this endpoint, which RequireHuman alone does not.
	stored, err := h.contacts.SetDomainAdmission(r.Context(), base, contacts.DomainAdmitted,
		"Kept from the open domain question: a colleague judged this domain a company.")
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, contacts.ToContractBlockedDomain(stored))
}

// DiscardDomainQuestion stops capturing a domain for the caller's own mailboxes.
//
// It writes the same rule POST /capture/exclusions writes with scope `user`,
// reached from the question rather than by typing the domain again. ONE
// COLLEAGUE'S ANSWER: excluding a domain for everybody is a different act with
// a different gate, and doing it from here would let any seat make a
// workspace-wide decision by answering their own mail.
//
// It does not destroy mail already captured — purging what a rule matched is
// its own irreversible endpoint.
func (h domainQuestionHandlers) DiscardDomainQuestion(w http.ResponseWriter, r *http.Request, domain string) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	// The REGISTRABLE domain, not what was typed. A rule stored as
	// `mail.example.com` binds that subdomain alone, so mail from the domain
	// the reader was actually asked about would keep arriving — the discard
	// would look like it worked and change nothing they can see.
	base, ok := freemail.Hostname(domain)
	if !ok {
		httperr.Write(w, r, httperr.Validation("domain", "invalid",
			"expected a domain name like example.com; a full email address or a URL is not one"))
		return
	}
	// Idempotent on the folded value: pressing discard twice answers the rule
	// that already exists rather than failing on the unique index.
	rule, err := h.exclusions.Add(r.Context(), capture.ExclusionScopeUser, capture.ExclusionKindDomain, base)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractExclusion(rule))
}
