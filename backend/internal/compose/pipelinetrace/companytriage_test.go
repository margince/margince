// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

// Every ledger state a company can be reached from renders as a rung that says
// something.
//
// The mapping is the whole of this door: the ledger's vocabulary is the
// engine's and the rung's is the member's, and a state that fell through to a
// bare status would be the silence the surface exists to remove — "pending"
// with no reason is an absence with extra steps.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	trace "github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
)

func TestEveryLedgerStateAReaderCanReachCarriesAReason(t *testing.T) {
	t.Parallel()

	decided := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		row    contacts.CompanyDomainTriage
		status trace.Status
		reason trace.Reason
	}{
		{
			// The ordinary answer: a site named the company and the record is it.
			name:   "a site identified the company",
			row:    contacts.CompanyDomainTriage{Status: contacts.DomainCompany},
			status: trace.StatusDone, reason: trace.ReasonCompanyWarranted,
		},
		{
			// Reachable from a company only on the arm that DID create one — a
			// parked landing page creates nothing and names no company to be
			// read from — so the sentence has to say which happened.
			name:   "no site identified one and the sender's name stood in",
			row:    contacts.CompanyDomainTriage{Status: contacts.DomainNoSite},
			status: trace.StatusDone, reason: trace.ReasonNoSiteIdentified,
		},
		{
			name: "held as a near duplicate",
			row: contacts.CompanyDomainTriage{
				Status: contacts.DomainPending, PendingReason: contacts.PendingNearDuplicate,
			},
			status: trace.StatusPending, reason: trace.ReasonTriageNearDupe,
		},
		{
			name: "nothing seen yet that would evidence one",
			row: contacts.CompanyDomainTriage{
				Status: contacts.DomainPending, PendingReason: contacts.PendingUnevidenced,
			},
			status: trace.StatusPending, reason: trace.ReasonTriageUnevidenced,
		},
		{
			name: "what was seen is too old",
			row: contacts.CompanyDomainTriage{
				Status: contacts.DomainPending, PendingReason: contacts.PendingStaleEvidence,
			},
			status: trace.StatusPending, reason: trace.ReasonTriageStale,
		},
		{
			// A pending row recording no reason. "Queued and not yet looked at"
			// is a different answer from "looked at and held", and a rung that
			// reported neither would say nothing at all.
			name:   "queued and not yet looked at",
			row:    contacts.CompanyDomainTriage{Status: contacts.DomainPending},
			status: trace.StatusPending, reason: trace.ReasonTriageQueued,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.row.DecidedAt = decided

			rung := triageRung(tc.row)

			if rung.Status != tc.status {
				t.Errorf("status = %q, want %q", rung.Status, tc.status)
			}
			if rung.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", rung.Reason, tc.reason)
			}
			if rung.At == nil || !rung.At.Equal(decided) {
				t.Errorf("at = %v, want the row's own %v — a member reads how long this has been "+
					"the answer, and a rung with no date cannot tell them", rung.At, decided)
			}
		})
	}
}

// The rung describes the SAME step the message ladder names, read from the one
// registration rather than restated. Two spellings of its order are how the two
// doors come to disagree about where in the pipeline it sits.
func TestTheRungIsTheRegisteredStage(t *testing.T) {
	t.Parallel()

	registration, ok := trace.Lookup(trace.StageCompanyTriage)
	if !ok {
		t.Fatal("company_triage is not registered, so this door describes a step the ladder does not have")
	}
	rung := triageRung(contacts.CompanyDomainTriage{Status: contacts.DomainCompany})

	if rung.Stage != registration.Stage || rung.Order != registration.Order ||
		rung.SubjectKind != registration.SubjectKind {
		t.Errorf("rung = %s/%d/%s, registration = %s/%d/%s",
			rung.Stage, rung.Order, rung.SubjectKind,
			registration.Stage, registration.Order, registration.SubjectKind)
	}
	// And the reasons this door emits are the closed set the registration
	// publishes — the client interpolates them into catalog keys, so one off
	// the list renders raw.
	for _, row := range []contacts.CompanyDomainTriage{
		{Status: contacts.DomainCompany},
		{Status: contacts.DomainNoSite},
		{Status: contacts.DomainPending},
		{Status: contacts.DomainPending, PendingReason: contacts.PendingUnevidenced},
		{Status: contacts.DomainPending, PendingReason: contacts.PendingStaleEvidence},
		{Status: contacts.DomainPending, PendingReason: contacts.PendingNearDuplicate},
	} {
		emitted := triageRung(row).Reason
		if !containsReason(registration.Reasons, emitted) {
			t.Errorf("this door emits %q, which the registration does not publish", emitted)
		}
	}
}

func containsReason(set []trace.Reason, want trace.Reason) bool {
	for _, r := range set {
		if r == want {
			return true
		}
	}
	return false
}

// A composition that wired no triage reader answers an ERROR, not an empty
// list. An empty list would say this company's domains were never triaged,
// which is a claim about the installation that a composition gap cannot make.
func TestACompositionWithNoTriageReaderSaysSoRatherThanReportingNone(t *testing.T) {
	t.Parallel()

	_, err := (&Assembler{}).ByCompanyID(context.Background(), ids.New[ids.CompanyKind]())

	if err == nil {
		t.Fatal("an unwired composition reported an answer about this company's domains")
	}
}
