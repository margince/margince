// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Every reason the operator's list can give, each written by its real writer.
// Each reads back as the code a client words in the reader's language.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// releaseQuestion clears an open question's owner, which is what puts it on
// the operator's list rather than on a colleague's queue.
func (e *dedupeEnv) releaseQuestion(ctx context.Context, t *testing.T, domain string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET owner_id = NULL WHERE domain = $1`, domain)
		return err
	}); err != nil {
		t.Fatalf("clearing the owner of %s: %v", domain, err)
	}
}

func (e *dedupeEnv) inTx(ctx context.Context, t *testing.T, what string, write func(tx pgx.Tx) error) {
	t.Helper()
	if err := e.store.tx(ctx, write); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

func (e *dedupeEnv) listedByDomain(ctx context.Context, t *testing.T) map[string]crmcontracts.BlockedDomain {
	t.Helper()
	entries, _, err := e.store.ListDomainAdmissions(ctx, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := map[string]crmcontracts.BlockedDomain{}
	for _, entry := range entries {
		out[entry.Domain] = ToContractBlockedDomain(entry)
	}
	return out
}

func TestEveryListedReasonTravelsAsACode(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	if _, err := e.store.SetDomainAdmission(ctx, "stated.example", DomainSuppressed, "A supplier we buy from"); err != nil {
		t.Fatalf("stating a refusal: %v", err)
	}
	e.inTx(ctx, t, "judging a bulk sender", func(tx pgx.Tx) error {
		return e.store.SuppressBulkSenderDomainTx(ctx, tx, "bulk.example", "newsletter")
	})
	if _, err := e.store.SetDomainAdmission(ctx, "claimed.example", DomainSuppressed, "Looked like a vendor"); err != nil {
		t.Fatalf("refusing the domain a colleague then claims: %v", err)
	}
	e.inTx(ctx, t, "claiming the domain for a company", func(tx pgx.Tx) error {
		return admitClaimedDomainTx(ctx, tx, "claimed.example")
	})

	e.withholdUnreadable(ctx, t, "hello@unevidenced.example", "unevidenced.example")
	e.releaseQuestion(ctx, t, "unevidenced.example")

	e.openTriage(ctx, t, "hello@stale.example", "", "stale.example")
	decadeAgo := time.Now().AddDate(-10, 0, 0)
	e.inTx(ctx, t, "withholding for stale evidence", func(tx pgx.Tx) error {
		withheld, err := withholdForStaleEvidence(ctx, tx,
			ResolveDomainTriageInput{Domain: "stale.example", Source: DomainSourceSiteRead},
			DomainDisposition{LastEvidenceAt: &decadeAgo})
		if err == nil && !withheld {
			t.Error("decade-old evidence was not withheld")
		}
		return err
	})
	e.releaseQuestion(ctx, t, "stale.example")

	e.openTriage(ctx, t, "hello@twin.example", "", "twin.example")
	e.inTx(ctx, t, "withholding for a name twin", func(tx pgx.Tx) error {
		return withholdForNearDuplicate(ctx, tx, ResolveDomainTriageInput{Domain: "twin.example"},
			CompanyCandidateScore{CandidateValue: "Twin", IncumbentValue: "Twin GmbH"})
	})
	e.releaseQuestion(ctx, t, "twin.example")

	listed := e.listedByDomain(ctx, t)
	want := map[string]crmcontracts.BlockedDomainReasonCode{
		"stated.example":      crmcontracts.BlockedDomainReasonCodeStated,
		"bulk.example":        crmcontracts.BlockedDomainReasonCodeBulkSender,
		"claimed.example":     crmcontracts.BlockedDomainReasonCodeCompanyCreated,
		"unevidenced.example": crmcontracts.BlockedDomainReasonCodeUnevidenced,
		"stale.example":       crmcontracts.BlockedDomainReasonCodeStaleEvidence,
		"twin.example":        crmcontracts.BlockedDomainReasonCodeNearDuplicate,
	}
	for domain, code := range want {
		got, ok := listed[domain]
		if !ok {
			t.Errorf("%s is not on the list %v", domain, listed)
			continue
		}
		if got.ReasonCode != code {
			t.Errorf("%s reason_code = %q, want %q", domain, got.ReasonCode, code)
		}
		if got.Reason == "" {
			t.Errorf("%s carries no reason sentence for clients that predate reason_code", domain)
		}
		if (got.Note != nil) != (code == crmcontracts.BlockedDomainReasonCodeStated) {
			t.Errorf("%s note = %v, want words only where somebody wrote them", domain, got.Note)
		}
	}
	if note := listed["stated.example"].Note; note == nil || *note != "A supplier we buy from" {
		t.Errorf("stated note = %v, want the decider's own words", note)
	}
	if kind := listed["bulk.example"].SenderKind; kind == nil || *kind != crmcontracts.BlockedDomainSenderKindNewsletter {
		t.Errorf("bulk sender_kind = %v, want newsletter", kind)
	}
}

// A company captured from mail is owner-private, so a colleague gets neither its
// id nor its name.
func TestAListedCompanyNameIsWithheldWithItsID(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	if _, err := e.store.SetDomainAdmission(ctx, "named.example", DomainSuppressed, "Not a customer"); err != nil {
		t.Fatalf("refusing the domain: %v", err)
	}
	company := ids.New[ids.CompanyKind]()
	e.inTx(ctx, t, "settling the domain onto an owner-private company", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO company (id, display_name, owner_id, source, captured_by, visibility)
			VALUES ($1, 'Named Co', $2, 'capture', 'connector:gmail', 'owner')`, company, e.rep); err != nil {
			return err
		}
		return adoptDispositionForCompany(ctx, tx, "named.example", company)
	})

	mine := e.listedByDomain(ctx, t)["named.example"]
	if mine.CompanyId == nil || mine.CompanyName == nil || *mine.CompanyName != "Named Co" {
		t.Fatalf("the owner's row = id %v, name %v, want the company named", mine.CompanyId, mine.CompanyName)
	}
	theirs, ok := e.listedByDomain(e.asOther(), t)["named.example"]
	if !ok {
		t.Fatal("the colleague's list does not carry the decision at all")
	}
	if theirs.CompanyId != nil || theirs.CompanyName != nil {
		t.Errorf("the colleague's row = id %v, name %v, want both withheld for a company they may not open",
			theirs.CompanyId, theirs.CompanyName)
	}
}
