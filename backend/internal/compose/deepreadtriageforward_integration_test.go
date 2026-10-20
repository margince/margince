// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestTriageForwardedAliasesRemainSeparateOpenQuestions(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	const destination = "https://destination.test/"
	for _, domain := range []string{"first-alias.test", "second-alias.test"} {
		args := openTriageQuestion(t, e, domain, "sales@"+domain, "Sales")
		requested := contacts.TriageSeedURL(domain)
		site := &fakeSite{pages: map[string]fakeSitePage{
			requested: {text: readable("Destination Corporation"), finalURL: destination},
		}}
		extract := &triageBrainFake{verdict: `{}`}
		triage := &triageBrainFake{verdict: `{"kind":"company","confidence":0.99}`}
		worker := newTriageTestWorker(e, site, extract, triage)
		if err := worker.run(ctx, args); err != nil {
			t.Fatal(err)
		}
		status, readStatus, companies := triageState(t, e, domain)
		if status != contacts.DomainPending || readStatus != "done" || companies != 0 {
			t.Fatalf("domain %s: status=%s read=%s companies=%d", domain, status, readStatus, companies)
		}
		if triage.calls != 0 || extract.calls != 0 {
			t.Fatal("forwarded identity reached classification or extraction")
		}
		var reason, raw string
		var bound bool
		var proposals int
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if err := tx.QueryRow(ctx, `SELECT pending_reason, evidence FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&reason, &raw); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT company_id IS NOT NULL FROM site_read WHERE id = $1`, args.SiteReadID).Scan(&bound); err != nil {
				return err
			}
			return tx.QueryRow(ctx, `SELECT count(*) FROM approval WHERE kind = $1`, siteLeadProposalKind).Scan(&proposals)
		})
		if err != nil {
			t.Fatal(err)
		}
		var evidence contacts.DomainForwardEvidence
		if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
			t.Fatal(err)
		}
		if reason != contacts.PendingForwardedIdentity || bound || proposals != 0 {
			t.Fatalf("reason=%s bound=%v proposals=%d", reason, bound, proposals)
		}
		if evidence.RequestedDomain != domain || evidence.EffectiveURL != destination || len(evidence.RedirectChain) != 2 {
			t.Fatalf("lost redirect provenance: %+v", evidence)
		}
	}
	questions, err := e.Contacts.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(questions) != 2 {
		t.Fatalf("questions=%d, want each alias separately", len(questions))
	}
}

func TestTriageSameDomainRedirectStillClassifiesTheRequestedIdentity(t *testing.T) {
	e := integration.Setup(t)
	args := openTriageQuestion(t, e, triageTestDomain, "person@"+triageTestDomain, "Personal Owner")
	site := &fakeSite{pages: map[string]fakeSitePage{seedURL: {text: readable("Personal Owner private homepage."), finalURL: "https://www." + triageTestDomain + "/"}}}
	extract := &triageBrainFake{verdict: `{}`}
	triage := &triageBrainFake{verdict: `{"kind":"personal","confidence":0.99}`}
	worker := newTriageTestWorker(e, site, extract, triage)
	if err := worker.run(e.As(e.Rep1, nil, integration.AdminPerms), args); err != nil {
		t.Fatal(err)
	}
	status, readStatus, companies := triageState(t, e, triageTestDomain)
	if status != contacts.DomainPersonal || readStatus != "cancelled" || companies != 0 || triage.calls != 1 {
		t.Fatalf("status=%s read=%s companies=%d classifications=%d", status, readStatus, companies, triage.calls)
	}
}

func TestTriageRedirectFailureRemainsAnUncertainQuestion(t *testing.T) {
	e := integration.Setup(t)
	args := openTriageQuestion(t, e, triageTestDomain, "sender@"+triageTestDomain, "Sender")
	site := &fakeSite{pageErrors: map[string][]error{seedURL: {errors.New("redirect loop")}}}
	extract := &triageBrainFake{verdict: `{}`}
	triage := &triageBrainFake{verdict: `{"kind":"company","confidence":0.99}`}
	worker := newTriageTestWorker(e, site, extract, triage)
	if err := worker.run(e.As(e.Rep1, nil, integration.AdminPerms), args); err != nil {
		t.Fatal(err)
	}
	status, readStatus, companies := triageState(t, e, triageTestDomain)
	if status != contacts.DomainPending || readStatus != "failed" || companies != 0 || triage.calls != 0 || extract.calls != 0 {
		t.Fatalf("status=%s read=%s companies=%d classifications=%d extractions=%d", status, readStatus, companies, triage.calls, extract.calls)
	}
}
