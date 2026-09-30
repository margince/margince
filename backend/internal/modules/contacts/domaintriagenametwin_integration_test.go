// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a company verdict does about a company that already carries the name
// the domain resolved to: one twin takes the domain, several hold the
// question, a close name creates and files a review pair, and the
// installation's own company is never a twin.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resolveSiteVerdict opens a domain's question with one sender and answers it
// the way the triage read does when the site states a company name.
func (e *dedupeEnv) resolveSiteVerdict(ctx context.Context, t *testing.T, sender, domain, stated string) ResolveDomainTriageResult {
	t.Helper()
	e.openTriageFirst(ctx, t, sender, "Some Sender", domain)
	return e.answerSiteVerdict(ctx, t, domain, stated, e.startTriageRead(ctx, t, domain))
}

// answerSiteVerdict answers an already-open question from one triage read. A
// test replays a verdict by passing the same read again, as a reclaimed job does.
func (e *dedupeEnv) answerSiteVerdict(ctx context.Context, t *testing.T, domain, stated string, readID ids.UUID) ResolveDomainTriageResult {
	t.Helper()
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: domain, Status: DomainCompany, Source: DomainSourceSiteRead,
		ReadID: readID, DossierName: stated, SeedURL: "https://" + domain,
	})
	if err != nil {
		t.Fatalf("resolving %s as %q: %v", domain, stated, err)
	}
	return res
}

// liveDomainsOf maps each of a company's live domains to whether it is primary.
func (e *dedupeEnv) liveDomainsOf(ctx context.Context, t *testing.T, id ids.CompanyID) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT domain, is_primary FROM company_domain
			 WHERE company_id = $1 AND archived_at IS NULL`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var domain string
			var primary bool
			if err := rows.Scan(&domain, &primary); err != nil {
				return err
			}
			out[domain] = primary
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the domains of %s: %v", id, err)
	}
	return out
}

// liveCompaniesNamed counts the live companies carrying exactly one name.
func (e *dedupeEnv) liveCompaniesNamed(ctx context.Context, t *testing.T, name string) int {
	t.Helper()
	var n int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM company WHERE display_name = $1 AND archived_at IS NULL`, name).Scan(&n)
	}); err != nil {
		t.Fatalf("counting companies named %q: %v", name, err)
	}
	return n
}

// The Baqend case: baqend.com and speedkit.com both resolved to "Baqend GmbH"
// and became two records four minutes apart.
func TestASecondDomainOfOneCompanyJoinsTheCompanyAlreadyHere(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	first := e.resolveSiteVerdict(ctx, t, "alice@baqend.test", "baqend.test", "Baqend GmbH")
	if !first.CompanyCreated || first.CompanyID == nil {
		t.Fatalf("the first domain = %+v, want its company created", first)
	}
	bob := e.openTriageFirst(ctx, t, "bob@speedkit.test", "Bob Brand", "speedkit.test")
	readID := e.startTriageRead(ctx, t, "speedkit.test")
	second := e.answerSiteVerdict(ctx, t, "speedkit.test", "Baqend GmbH", readID)

	if second.CompanyCreated || second.CompanyID == nil || *second.CompanyID != *first.CompanyID {
		t.Fatalf("the second domain = %+v, want it on the existing company %s", second, first.CompanyID)
	}
	if n := e.liveCompaniesNamed(ctx, t, "Baqend GmbH"); n != 1 {
		t.Fatalf("%d companies named Baqend GmbH, want 1", n)
	}
	// The company keeps the primary domain it was created with.
	domains := e.liveDomainsOf(ctx, t, *first.CompanyID)
	if len(domains) != 2 || !domains["baqend.test"] || domains["speedkit.test"] {
		t.Fatalf("company domains = %v, want baqend.test primary and speedkit.test secondary", domains)
	}
	if status, _, _, _, companyID := e.dispositionRow(ctx, t, "speedkit.test"); status != DomainCompany ||
		companyID == nil || *companyID != *first.CompanyID {
		t.Fatalf("speedkit.test settled as %q on %v, want %q on %s", status, companyID, DomainCompany, first.CompanyID)
	}
	if second.EdgesPlanted != 1 {
		t.Fatalf("the adoption planted %d employment edges, want 1 for the sender waiting on it", second.EdgesPlanted)
	}
	if employer := e.employerOf(ctx, t, bob.ContactID); employer == nil || *employer != first.CompanyID.UUID {
		t.Fatalf("the speedkit sender is employed at %v, want %s", employer, first.CompanyID)
	}
	// The decision is on the record's trail as a change to its domains.
	before, after := auditImagesHolding(ctx, t, e.store, entityCompany, first.CompanyID.UUID, auditKeyDomains)
	if !slices.Equal(imageDomains(t, before), []string{"baqend.test"}) ||
		!slices.Equal(imageDomains(t, after), []string{"baqend.test", "speedkit.test"}) {
		t.Fatalf("audit images = %v -> %v, want the domain list before and after the adoption", before, after)
	}

	// Asked again, the answer is the same company and nothing is written twice.
	replay := e.answerSiteVerdict(ctx, t, "speedkit.test", "Baqend GmbH", readID)
	if replay.CompanyCreated || replay.CompanyID == nil || *replay.CompanyID != *first.CompanyID {
		t.Fatalf("the replay = %+v, want the same company and no create", replay)
	}
	if n := e.countCompaniesOn(ctx, t, "speedkit.test"); n != 1 {
		t.Fatalf("%d company domains for speedkit.test after the replay, want 1", n)
	}
	if n := countAuditRowsHolding(ctx, t, e.store, entityCompany, first.CompanyID.UUID, auditKeyDomains); n != 1 {
		t.Fatalf("%d adoption audit rows after the replay, want 1", n)
	}

	// The next sender on the adopted domain lands on the company directly.
	carol, err := e.store.EnsureCounterparty(ctx, e.ensureInput(ctx, t, "carol@speedkit.test", "Carol Cole", "speedkit.test"))
	if err != nil {
		t.Fatalf("ensure after the adoption: %v", err)
	}
	if carol.TriagePending || carol.CompanyID == nil || *carol.CompanyID != *first.CompanyID {
		t.Fatalf("ensure after the adoption = %+v, want the existing company", carol)
	}
}

// imageDomains reads the sorted domain list out of a decoded audit image.
func imageDomains(t *testing.T, image map[string]any) []string {
	t.Helper()
	list, ok := image[auditKeyDomains].([]any)
	if !ok {
		t.Fatalf("audit image %v carries no domain list", image)
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		domain, ok := v.(string)
		if !ok {
			t.Fatalf("audit image domain %v is not a string", v)
		}
		out = append(out, domain)
	}
	slices.Sort(out)
	return out
}

// A bare brand is not the same name as the legal entity. The dedupe key keeps
// the legal form, so "Baqend" gets its own record, and the pair goes to the
// review queue for a human to merge.
func TestABareBrandNameCreatesAndFilesThePairForReview(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	first := e.resolveSiteVerdict(ctx, t, "alice@baqend.test", "baqend.test", "Baqend GmbH")
	second := e.resolveSiteVerdict(ctx, t, "bob@speedkit.test", "speedkit.test", "Baqend")

	if !second.CompanyCreated || second.CompanyID == nil || *second.CompanyID == *first.CompanyID {
		t.Fatalf("the bare brand = %+v, want a record of its own", second)
	}
	if _, joined := e.liveDomainsOf(ctx, t, *first.CompanyID)["speedkit.test"]; joined {
		t.Fatal("a close name gave its domain to the existing company")
	}
	if n := e.companyCandidatesFor(ctx, t, *second.CompanyID); n != 1 {
		t.Fatalf("%d review pairs for the bare brand, want 1 against Baqend GmbH", n)
	}
}

func TestADifferentNameCreatesItsOwnCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	first := e.resolveSiteVerdict(ctx, t, "alice@baqend.test", "baqend.test", "Baqend GmbH")
	second := e.resolveSiteVerdict(ctx, t, "bob@speedkit.test", "speedkit.test", "Speedkit Labs GmbH")

	if !second.CompanyCreated || second.CompanyID == nil || *second.CompanyID == *first.CompanyID {
		t.Fatalf("a different name = %+v, want a record of its own", second)
	}
	if n := e.companyCandidatesFor(ctx, t, *second.CompanyID); n != 0 {
		t.Fatalf("%d review pairs for an unrelated name, want 0", n)
	}
}

// Two companies already carry the name, so choosing one would be choosing by
// uuid. Nothing is created and the question waits for a human.
func TestSeveralCompaniesOfOneNameHoldTheQuestion(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	for _, domain := range []string{"twin-one.test", "twin-two.test"} {
		if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
			DisplayName: "Twin Works GmbH", Source: "manual",
			Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
		}); err != nil {
			t.Fatalf("seeding the twin on %s: %v", domain, err)
		}
	}
	sender := e.openTriageFirst(ctx, t, "dana@twin-three.test", "Dana Dorn", "twin-three.test")
	res := e.answerSiteVerdict(ctx, t, "twin-three.test", "Twin Works GmbH", e.startTriageRead(ctx, t, "twin-three.test"))

	if res.CompanyID != nil || res.CompanyCreated {
		t.Fatalf("the verdict = %+v, want the question held", res)
	}
	if n := e.liveCompaniesNamed(ctx, t, "Twin Works GmbH"); n != 2 {
		t.Fatalf("%d companies named Twin Works GmbH, want the 2 seeded", n)
	}
	if n := e.countCompaniesOn(ctx, t, "twin-three.test"); n != 0 {
		t.Fatalf("%d companies hold twin-three.test, want 0", n)
	}
	if status, reason, _, _, _ := e.dispositionRow(ctx, t, "twin-three.test"); status != DomainPending || reason != PendingNearDuplicate {
		t.Fatalf("disposition = %q/%q, want %q/%q", status, reason, DomainPending, PendingNearDuplicate)
	}
	if employer := e.employerOf(ctx, t, sender.ContactID); employer != nil {
		t.Fatalf("the held domain's sender is employed at %s, want no employer yet", *employer)
	}
}

// The installation's own company is never a name twin. Adopting onto it would
// make the new domain one of the workspace's own, and its senders colleagues.
func TestADomainNamedLikeTheOwnCompanyDoesNotJoinIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	website := "https://ourhouse.test"
	anchor, err := e.store.SaveCompany(e.asAdmin(), SaveCompanyInput{DisplayName: "Our House GmbH", Website: &website})
	if err != nil {
		t.Fatalf("seeding the own company: %v", err)
	}
	res := e.resolveSiteVerdict(ctx, t, "eve@lookalike.test", "lookalike.test", "Our House GmbH")

	if !res.CompanyCreated || res.CompanyID == nil || *res.CompanyID == anchor.CompanyID {
		t.Fatalf("the verdict = %+v, want a record of its own beside the own company", res)
	}
	if domains := e.liveDomainsOf(ctx, t, anchor.CompanyID); len(domains) != 1 || !domains["ourhouse.test"] {
		t.Fatalf("own company domains = %v, want only ourhouse.test", domains)
	}
}

// A twin the resolving seat may not write receives nothing: the verdict is
// refused, no domain lands on the twin, and no second record is minted.
func TestATwinTheSeatCannotWriteIsNotJoined(t *testing.T) {
	e := setupDedupe(t)

	twin, err := e.store.CreateCompany(e.asOther(), CreateCompanyInput{
		DisplayName: "Guarded Works GmbH", Source: "manual",
		Domains: []CompanyDomainInput{{Domain: "guarded-one.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("seeding the colleague's company: %v", err)
	}
	ctx := e.asOwnRows()
	e.openTriageFirst(ctx, t, "finn@guarded-two.test", "Finn Frey", "guarded-two.test")
	_, err = e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "guarded-two.test", Status: DomainCompany, Source: DomainSourceSiteRead,
		ReadID: e.startTriageRead(ctx, t, "guarded-two.test"), DossierName: "Guarded Works GmbH",
		SeedURL: "https://guarded-two.test",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the verdict answered %v, want ErrPermissionDenied — the twin is not this seat's to change", err)
	}
	twinID := ids.From[ids.CompanyKind](ids.UUID(twin.Id))
	if domains := e.liveDomainsOf(ctx, t, twinID); len(domains) != 1 {
		t.Fatalf("the colleague's company holds %v, want only its own domain", domains)
	}
	if n := e.countCompaniesOn(ctx, t, "guarded-two.test"); n != 0 {
		t.Fatalf("%d companies hold guarded-two.test, want 0", n)
	}
}

// asAdmin is the rep's seat with the administrator role, for the own-company
// profile only an administrator may write.
func (e *dedupeEnv) asAdmin() context.Context {
	ctx := e.as()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RoleKeys = []string{"admin"}
	// Saving the website claims its domain as one of the workspace's own.
	actor.Permissions.Objects["capture_settings"] = principal.ObjectGrant{Read: true, Update: true}
	return principal.WithActor(ctx, actor)
}

// asOwnRows is the rep's seat confined to the rows it owns.
func (e *dedupeEnv) asOwnRows() context.Context {
	ctx := e.as()
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RowScope = principal.RowScopeOwn
	return principal.WithActor(ctx, actor)
}
