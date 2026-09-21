// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

// The company-keyed door onto the ladder's one domain-subject stage.
//
// The other two doors are message-keyed, and this stage does not fit either:
// its subject is a DOMAIN, triaged once for every message that ever arrives
// from it. A per-message rung would answer "done" for the message that prompted
// the triage and for the hundredth one after it alike, which reads as this
// message having been the cause. So the message ladder names where the answer
// lives (AbsentAnsweredOnTheCompany) and the answer itself is here.
//
// It does not re-derive the triage rule, for the reason the package header
// gives: the verdict is contacts' to decide and this reads what it decided.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	trace "github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
)

// DomainTriageReader is the ledger read this door needs, as an interface for
// the reason the thread reader beside it is one: the module owns the rows and
// this package owns the rendering, and a compose package that took the store
// concretely would make every test of the rendering need a database.
type DomainTriageReader interface {
	DomainTriageForCompany(ctx context.Context, id ids.CompanyID) ([]contacts.CompanyDomainTriage, error)
}

// WithDomainTriageReader supplies it. Optional like the thread reader: a
// composition without it has no triage to report, and ByCompanyID says so
// rather than reporting an empty answer as "nothing was triaged".
func (a *Assembler) WithDomainTriageReader(r DomainTriageReader) *Assembler {
	a.domains = r
	return a
}

// errNoDomainTriageReader is a composition that wired no triage reader. It is
// an error rather than an empty answer for the reason every other absence on
// this surface is named: an empty list here would say this company's domains
// were never triaged, which is a claim about the installation that a
// composition gap cannot support.
var errNoDomainTriageReader = errors.New("compose: this deployment composed no domain triage, so there is no company check to read")

// DomainRung is one domain's triage answer.
type DomainRung struct {
	Domain string
	Rung   Rung
}

// CompanyTriage is the whole answer for one company.
type CompanyTriage struct {
	CompanyID ids.CompanyID
	Domains   []DomainRung
}

// ByCompanyID answers which domains were triaged into this company, and what
// each concluded.
//
// The caller's gate is the COMPANY's, applied by the store: a company outside
// the reader's row scope is existence-hidden there rather than answered with an
// empty list here.
func (a *Assembler) ByCompanyID(ctx context.Context, id ids.CompanyID) (CompanyTriage, error) {
	if a.domains == nil {
		return CompanyTriage{}, errNoDomainTriageReader
	}
	rows, err := a.domains.DomainTriageForCompany(ctx, id)
	if err != nil {
		return CompanyTriage{}, err
	}
	out := CompanyTriage{CompanyID: id, Domains: make([]DomainRung, 0, len(rows))}
	for _, row := range rows {
		out.Domains = append(out.Domains, DomainRung{Domain: row.Domain, Rung: triageRung(row)})
	}
	return out, nil
}

// triageRung maps one ledger row onto the rung vocabulary.
//
// EVERY ARM CARRIES A REASON, including the ones that concluded. A member
// reading "done" about their own company learns nothing they did not already
// know from the company existing; what they came for is which of the two ways
// it came to exist — a site that named it, or a site that named nothing and a
// sender's own name standing in.
//
// The pending arms are the ledger's own pending_reason vocabulary, and the
// default arm is deliberately a reason rather than a blank: a domain queued and
// not yet looked at is a different answer from one looked at and held, and a
// rung that reported neither would be the silence this surface removes.
func triageRung(row contacts.CompanyDomainTriage) Rung {
	// Stage, order and subject come from the ONE registration, never restated:
	// this door and the message ladder must describe the same step, and a
	// second spelling of its order is how the two would come to disagree about
	// where in the pipeline it sits.
	registration, _ := trace.Lookup(trace.StageCompanyTriage)
	rung := Rung{
		Stage:       registration.Stage,
		Order:       registration.Order,
		SubjectKind: registration.SubjectKind,
		At:          &row.DecidedAt,
	}
	switch row.Status {
	case contacts.DomainCompany:
		rung.Status, rung.Reason = trace.StatusDone, trace.ReasonCompanyWarranted
	case contacts.DomainNoSite:
		// Reachable from a company only on the arm that DID create one: a
		// parked landing page creates nothing and so names no company to be
		// read from. The sentence says which of those happened.
		rung.Status, rung.Reason = trace.StatusDone, trace.ReasonNoSiteIdentified
	default:
		rung.Status, rung.Reason = trace.StatusPending, pendingReason(row.PendingReason)
	}
	return rung
}

// pendingReason renders the ledger's own pending vocabulary, defaulting to
// "queued" for a row that records none.
func pendingReason(recorded string) trace.Reason {
	switch recorded {
	case contacts.PendingUnevidenced:
		return trace.ReasonTriageUnevidenced
	case contacts.PendingStaleEvidence:
		return trace.ReasonTriageStale
	case contacts.PendingNearDuplicate:
		return trace.ReasonTriageNearDupe
	default:
		return trace.ReasonTriageQueued
	}
}
