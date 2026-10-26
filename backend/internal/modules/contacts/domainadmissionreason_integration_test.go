// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// Every reason the operator's list can give, each written by its real writer.
// Each reads back as the code a client words in the reader's language.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// releaseQuestion clears an open question's owner. The question stays on the
// operator's list, where every open question is.
func (e *dedupeEnv) releaseQuestion(ctx context.Context, t *testing.T, domain string) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM company_domain_question_reader WHERE domain = $1`, domain); err != nil {
			return err
		}
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
	if _, err := e.store.KeepDomainAsCompany(ctx, "kept.example"); err != nil {
		t.Fatalf("keeping an open question as a company: %v", err)
	}
	e.inTx(ctx, t, "changing display text without changing the decision", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET admission_reason=@reason
 WHERE domain = ANY(@domains)`, pgx.NamedArgs{
			"reason":  "Unveränderte Entscheidung, andere Formulierung.",
			"domains": []string{"bulk.example", "claimed.example", "kept.example"},
		})
		return err
	})
	// No writer stamps heuristic today, and the schema still admits it.
	e.inTx(ctx, t, "recording a heuristic refusal", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company_domain_disposition (domain, admission, admission_reason, admission_source, admission_at)
			VALUES ('guessed.example', 'suppressed', 'looked like a mailing list', 'heuristic', now())`)
		return err
	})

	e.withholdUnreadable(ctx, t, "hello@unevidenced.example", "unevidenced.example")
	e.releaseQuestion(ctx, t, "unevidenced.example")

	e.openTriage(ctx, t, "hello@stale.example", "", "stale.example")
	decadeAgo := clocktest.Now(t).AddDate(-10, 0, 0)
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
		"kept.example":        crmcontracts.BlockedDomainReasonCodeKeptAsCompany,
		"guessed.example":     crmcontracts.BlockedDomainReasonCodeUnknown,
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
	e.settleOnPrivateCompany(ctx, t, "named.example", "Named Co")

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

// The open question a name twin leaves reaches its owner with the same sentence
// the operator's list gives it once nobody owns it.
func TestANameTwinQuestionTellsItsOwnerWhy(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriage(ctx, t, "hello@twin.example", "", "twin.example")
	e.inTx(ctx, t, "withholding for a name twin", func(tx pgx.Tx) error {
		return withholdForNearDuplicate(ctx, tx, ResolveDomainTriageInput{Domain: "twin.example"},
			CompanyCandidateScore{CandidateValue: "Twin", IncumbentValue: "Twin GmbH"})
	})

	backlog, err := e.store.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatalf("owner backlog: %v", err)
	}
	if len(backlog) != 1 || backlog[0].Domain != "twin.example" || backlog[0].Reason == "" {
		t.Fatalf("owner backlog = %+v, want twin.example with the reason it was left open", backlog)
	}
	e.releaseQuestion(ctx, t, "twin.example")
	if listed := e.listedByDomain(ctx, t)["twin.example"]; listed.Reason != backlog[0].Reason {
		t.Errorf("operator list reason %q, owner backlog reason %q, want one sentence on both", listed.Reason, backlog[0].Reason)
	}
}

// privateCompany is a company captured from the first rep's mail, which stays
// private to them, so the second rep may not open it.
func (e *dedupeEnv) privateCompany(ctx context.Context, t *testing.T, name string) ids.CompanyID {
	t.Helper()
	company := ids.New[ids.CompanyKind]()
	e.inTx(ctx, t, "capturing an owner-private company", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company (id, display_name, owner_id, source, captured_by, visibility)
			VALUES ($1, $2, $3, 'capture', 'connector:gmail', 'owner')`, company, name, e.rep)
		return err
	})
	return company
}

func (e *dedupeEnv) settleOnPrivateCompany(ctx context.Context, t *testing.T, domain, name string) {
	t.Helper()
	company := e.privateCompany(ctx, t, name)
	e.inTx(ctx, t, "settling the domain onto the company", func(tx pgx.Tx) error {
		return adoptDispositionForCompany(ctx, tx, domain, company)
	})
}

// A colleague may decide a domain or re-ask its question without learning which
// private company it sits on: each write answers with the company withheld.
func TestADecisionAnswersWithoutACompanyTheCallerMayNotOpen(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriage(ctx, t, "hello@decided.example", "", "decided.example")
	e.settleOnPrivateCompany(ctx, t, "decided.example", "Private Co")

	mine, err := e.store.SetDomainAdmission(ctx, "decided.example", DomainSuppressed, "Not a customer")
	if err != nil {
		t.Fatalf("the owner's decision: %v", err)
	}
	if mine.CompanyID == nil || mine.CompanyName == nil {
		t.Fatalf("the owner's read-back = id %v, name %v, want the company named", mine.CompanyID, mine.CompanyName)
	}
	theirs, err := e.store.SetDomainAdmission(e.asOther(), "decided.example", DomainSuppressed, "Still not a customer")
	if err != nil {
		t.Fatalf("the colleague's decision: %v", err)
	}
	if theirs.CompanyID != nil || theirs.CompanyName != nil {
		t.Errorf("the colleague's read-back = id %v, name %v, want both withheld", theirs.CompanyID, theirs.CompanyName)
	}

	e.withholdUnreadable(ctx, t, "hello@reasked.example", "reasked.example")
	company := e.privateCompany(ctx, t, "Elsewhere Co")
	// No writer leaves a company on an open question, and the schema allows one.
	e.inTx(ctx, t, "pointing the open question at the private company", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET company_id = $1 WHERE domain = 'reasked.example'`, company)
		return err
	})
	reasked, err := e.store.ReopenWithheldDomain(e.asOther(), "reasked.example")
	if err != nil {
		t.Fatalf("the colleague's re-ask: %v", err)
	}
	if reasked.CompanyID != nil || reasked.CompanyName != nil {
		t.Errorf("the colleague's re-ask = id %v, name %v, want both withheld", reasked.CompanyID, reasked.CompanyName)
	}
}
