// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// How FRESH a domain's evidence is, over a real Postgres.
//
// A triage crawl reads the site as it stands today, so the age of the newest
// mail decides whether today's site may answer for it at all. These tests cover
// what is recorded, what withholds a company, what rearms a withheld question —
// and the review pair capture files when it is about to mint a second contact
// spelled exactly like one the workspace already has.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// lastEvidenceAt reads the age the disposition recorded for a domain.
func (e *dedupeEnv) lastEvidenceAt(ctx context.Context, t *testing.T, domain string) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT last_evidence_at FROM company_domain_disposition WHERE domain = $1`,
			domain).Scan(&at)
	}); err != nil {
		t.Fatalf("reading the evidence age of %s: %v", domain, err)
	}
	return at
}

// A crawl reads the site as it stands TODAY. A domain whose newest mail is a
// decade old does not argue that this company is there now — it changed hands,
// or the business closed — so the verdict is withheld for a human rather than
// attaching a decade of contacts to whoever owns the domain currently.
func TestStaleEvidenceWithholdsTheCompanyInsteadOfMintingIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	old := time.Now().AddDate(-(staleEvidenceYears + 2), 0, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "hans@gonecorp.test", "gonecorp.test", old)); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if at := e.lastEvidenceAt(ctx, t, "gonecorp.test"); at == nil {
		t.Fatal("the disposition recorded no evidence age; the gate would read every domain as fresh")
	}

	readID := e.startTriageRead(ctx, t, "gonecorp.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "gonecorp.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Gonecorp GmbH", SeedURL: "https://gonecorp.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.CompanyCreated || res.CompanyID != nil {
		t.Fatalf("resolve = %+v, want NO company from a decade-old thread", res)
	}
	if n := e.countCompaniesOn(ctx, t, "gonecorp.test"); n != 0 {
		t.Fatalf("%d companies on a domain nobody has written from in years, want 0", n)
	}
	status, reason, _, _, companyID := e.dispositionRow(ctx, t, "gonecorp.test")
	if status != DomainPending {
		t.Errorf("status = %q, want it left open — a withheld domain must stay askable", status)
	}
	if reason != "stale_evidence" {
		t.Errorf("pending_reason = %q, want stale_evidence — the row must say WHY it has no company", reason)
	}
	if companyID != nil {
		t.Errorf("company_id = %v, want none", companyID)
	}
}

// Two contacts written exactly the same way are worth a human's glance, and
// capture had no way to raise one: the name-collision lane existed and only the
// manual create and the vcard import opted in. A decade of mail puts two
// contacts both called "Michael Schmidt" in one workspace in silence.
//
// The pair is FILED, never merged and never refused — a father and son at one
// firm are two real records that share a name.
func TestCapturingASecondContactOfTheSameNameFilesAReviewPair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, "m.schmidt@firstco.test", "Michael Schmidt", "firstco.test"))
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	// A different address at a different company, spelled the same way. Nothing
	// else about the two agrees, so only the name lane can raise this.
	second, err := e.store.EnsureCounterparty(ctx,
		e.ensureInput(ctx, t, "michael@secondco.test", "Michael Schmidt", "secondco.test"))
	if err != nil {
		t.Fatalf("second ensure: %v", err)
	}
	if !second.ContactCreated || second.ContactID == first.ContactID {
		t.Fatalf("second ensure = %+v, want a SEPARATE contact — the lane flags, it never merges", second)
	}

	pairs := openCandidatePairs(ctx, t, e, entityContact, second.ContactID.UUID)
	if len(pairs) != 1 {
		t.Fatalf("%d open review pairs for the second Michael Schmidt, want exactly 1", len(pairs))
	}
	if pairs[0].OtherID != first.ContactID.String() {
		t.Errorf("the pair names %s, want the incumbent %s", pairs[0].OtherID, first.ContactID)
	}
	// An exact spelling is not a probability. A score beside it would invite a
	// reviewer to read a confidence the lane never computed.
	if pairs[0].Confidence != 0 {
		t.Errorf("confidence = %v, want 0 — the name lane scores nothing", pairs[0].Confidence)
	}
}

// A header's date is the sender's to type. Without a floor under the age, one
// message claiming to be from 2040 reads as the freshest evidence on the domain
// and waves the whole question through — the one direction this gate must not
// fail in.
func TestAFutureDatedHeaderDoesNotPassAsFreshEvidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	old := time.Now().AddDate(-(staleEvidenceYears + 2), 0, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "hans@forged.test", "forged.test", old)); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	// The forged message. It is the newest thing on the domain by its own claim.
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "spoof@forged.test", "forged.test", time.Now().AddDate(15, 0, 0))); err != nil {
		t.Fatalf("forged ensure: %v", err)
	}

	readID := e.startTriageRead(ctx, t, "forged.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "forged.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Forged GmbH", SeedURL: "https://forged.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.CompanyCreated || res.CompanyID != nil {
		t.Fatalf("resolve = %+v, want NO company — a date in the future is not evidence of a live business", res)
	}
	if _, reason, _, _, _ := e.dispositionRow(ctx, t, "forged.test"); reason != "stale_evidence" {
		t.Errorf("pending_reason = %q, want stale_evidence", reason)
	}
}

// Evidence is dated from every address UNDER the domain, not only from the
// registrable domain itself. freemail.Hostname narrows to eTLD+1, so a match on
// the address's exact host would drop `alice@mail.acme.test` — and dropping
// recent senders is the dangerous direction: the max then comes from an older
// message and withholds a company the recent mail had earned.
func TestASubdomainSenderIsEvidenceAboutTheDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	old := time.Now().AddDate(-(staleEvidenceYears + 2), 0, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "hans@subco.test", "subco.test", old)); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	// Somebody at a subdomain writes today. The disposition is keyed on the
	// registrable domain, so this is the same question — and it is fresh.
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "alice@mail.subco.test", "mail.subco.test", time.Now())); err != nil {
		t.Fatalf("subdomain ensure: %v", err)
	}

	readID := e.startTriageRead(ctx, t, "subco.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "subco.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Subco GmbH", SeedURL: "https://subco.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !res.CompanyCreated || res.CompanyID == nil {
		t.Fatalf("resolve = %+v, want the company created — a subdomain sender wrote today", res)
	}
}

// New mail dates the domain even when the question is merely OPEN, not withheld.
// The reopen is guarded on the withheld state, so without a write on every
// message the crawl reads a stale timestamp, withholds the domain and clears the
// cursor — while the newer mail that should have rearmed it was never recorded.
func TestMailOnAnOpenQuestionStillDatesTheDomain(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	old := time.Now().AddDate(-(staleEvidenceYears + 2), 0, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "hans@stillopen.test", "stillopen.test", old)); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	// The question is open and NOT withheld — no crawl has answered yet.
	if _, reason, _, _, _ := e.dispositionRow(ctx, t, "stillopen.test"); reason != "" {
		t.Fatalf("pending_reason = %q, want the question merely open", reason)
	}
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "neu@stillopen.test", "stillopen.test", time.Now())); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	at := e.lastEvidenceAt(ctx, t, "stillopen.test")
	if at == nil || time.Since(*at) > staleEvidenceYears*365*24*time.Hour {
		t.Fatalf("last_evidence_at = %v, want today's mail recorded while the question was open", at)
	}
	// And the crawl therefore answers rather than withholding.
	readID := e.startTriageRead(ctx, t, "stillopen.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "stillopen.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Stillopen GmbH", SeedURL: "https://stillopen.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !res.CompanyCreated {
		t.Fatalf("resolve = %+v, want the company created from the newer mail", res)
	}
}

// The ordinary case is untouched: recent mail still mints the company. Without
// this the gate could withhold everything and every other test here would still
// pass, because they all leave occurred_at NULL.
func TestRecentEvidenceStillMintsTheCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	recent := time.Now().AddDate(0, -2, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "ida@livecorp.test", "livecorp.test", recent)); err != nil {
		t.Fatalf("ensure: %v", err)
	}

	readID := e.startTriageRead(ctx, t, "livecorp.test")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "livecorp.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Livecorp GmbH", SeedURL: "https://livecorp.test",
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !res.CompanyCreated || res.CompanyID == nil {
		t.Fatalf("resolve = %+v, want the company created from recent mail", res)
	}
	if _, reason, _, _, _ := e.dispositionRow(ctx, t, "livecorp.test"); reason != "" {
		t.Errorf("pending_reason = %q, want none — the question was answered", reason)
	}
}

// Withholding is temporary, not a quiet refusal. Somebody writing from the
// domain again is exactly the evidence that was missing, so the question
// reopens and the next crawl may answer it.
func TestNewMailReopensADomainWithheldForStaleEvidence(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	old := time.Now().AddDate(-(staleEvidenceYears + 2), 0, 0)
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "hans@revived.test", "revived.test", old)); err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	readID := e.startTriageRead(ctx, t, "revived.test")
	if _, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "revived.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		Evidence: "the site states a legal entity", ReadID: readID,
		DossierName: "Revived GmbH", SeedURL: "https://revived.test",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, reason, _, _, _ := e.dispositionRow(ctx, t, "revived.test"); reason != "stale_evidence" {
		t.Fatalf("pending_reason = %q, want the domain withheld first", reason)
	}

	// Somebody there writes today.
	if _, err := e.store.EnsureCounterparty(ctx,
		e.datedEnsureInput(ctx, t, "neu@revived.test", "revived.test", time.Now())); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	if _, reason, _, _, _ := e.dispositionRow(ctx, t, "revived.test"); reason != "" {
		t.Errorf("pending_reason = %q, want it cleared — the question is live again", reason)
	}
	// The CURSOR is what the sweep reads, and rearming it is what reopening
	// means. dueContains is deliberately not asserted here: this test started a
	// triage read that is still in flight, and ListDueDomains excludes a domain
	// whose crawl is running so the budget is not spent twice. That exclusion
	// lifts on its own when the read finishes or goes stale.
	var rearmed bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT next_attempt_at IS NOT NULL AND next_attempt_at <= now()
			  FROM company_domain_disposition WHERE domain = $1`, "revived.test").Scan(&rearmed)
	}); err != nil {
		t.Fatal(err)
	}
	if !rearmed {
		t.Fatal("new mail did not rearm the withheld question; the domain would wait for ever")
	}
	if at := e.lastEvidenceAt(ctx, t, "revived.test"); at == nil ||
		time.Since(*at) > staleEvidenceYears*365*24*time.Hour {
		t.Fatalf("last_evidence_at = %v, want today's mail recorded as the newest evidence", at)
	}
}
