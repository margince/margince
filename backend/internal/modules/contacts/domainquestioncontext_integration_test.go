// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What an open company question tells its reader. It names who wrote and counts
// the contacts a confirmed company employs. It says whether the site was read,
// and which existing company the evidence points to.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// questionAbout is the reader's open question about one domain, or a failure.
func (e *dedupeEnv) questionAbout(ctx context.Context, t *testing.T, domain string) DomainQuestion {
	t.Helper()
	rows, err := e.store.OpenDomainQuestionsForOwner(ctx)
	if err != nil {
		t.Fatalf("reading open domain questions: %v", err)
	}
	for _, row := range rows {
		if row.Domain == domain {
			return row
		}
	}
	t.Fatalf("no open question about %s among %d", domain, len(rows))
	return DomainQuestion{}
}

func (e *dedupeEnv) candidateOf(ctx context.Context, t *testing.T, domain string) *ids.UUID {
	t.Helper()
	var candidate *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT candidate_company_id FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&candidate)
	}); err != nil {
		t.Fatalf("reading the candidate of %s: %v", domain, err)
	}
	return candidate
}

func TestAQuestionNamesItsSenderAndWhetherTheSiteWasRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "anna@unread-site.test", "Anna Example", "unread-site.test")
	e.openTriageFirst(ctx, t, "ben@read-site.test", "Ben Example", "read-site.test")
	for domain, unread := range map[string]bool{"unread-site.test": true, "read-site.test": false} {
		if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: domain, Evidence: "no company", SiteUnread: unread}); err != nil {
			t.Fatalf("withholding %s: %v", domain, err)
		}
	}

	unread := e.questionAbout(ctx, t, "unread-site.test").Context
	if len(unread.Contacts) != 1 || unread.Contacts[0].Name != "Anna Example" || unread.ContactCount != 1 {
		t.Fatalf("context = %+v, want Anna named and counted", unread)
	}
	if unread.OriginActivityID == nil || unread.SenderAddress != "anna@unread-site.test" || unread.HiddenSenders {
		t.Fatalf("context = %+v, want the mail Anna sent as its origin", unread)
	}
	if !unread.SiteUnread || unread.Suggestion != nil {
		t.Fatalf("context = %+v, want an unread site and no suggestion", unread)
	}
	if read := e.questionAbout(ctx, t, "read-site.test").Context; read.SiteUnread {
		t.Fatal("a site that was read and named no company is reported as never read")
	}
}

func TestARedirectedSiteSuggestsTheCompanyOwningItsTarget(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	target := e.resolveSiteVerdict(ctx, t, "target@example-company.test", "example-company.test", "Example Company")
	e.openTriageFirst(ctx, t, "alex@alias.test", "Alex Example", "alias.test")
	if err := e.store.WithholdForwardedDomain(ctx, DomainForwardEvidence{
		ReadID: e.startTriageRead(ctx, t, "alias.test"), RequestedDomain: "alias.test",
		EffectiveURL: "https://www.example-company.test/", RedirectChain: []string{"https://alias.test/", "https://www.example-company.test/"},
	}); err != nil {
		t.Fatal(err)
	}

	got := e.questionAbout(ctx, t, "alias.test").Context.Suggestion
	if got == nil || got.CompanyID != target.CompanyID.UUID || got.Basis != SuggestionRedirectTarget || got.Name != "Example Company" {
		t.Fatalf("suggestion = %+v, want Example Company from the redirect target", got)
	}
	// The suggestion links nothing: keeping still needs the company named.
	if _, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{Domain: "alias.test", Status: DomainCompany, Source: DomainSourceHuman}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("keeping without a company = %v, want a conflict", err)
	}
	focused, _, err := e.store.DomainQuestionsForOwner(ctx, "", 50, "alias.test")
	if err != nil || len(focused) != 1 || focused[0].Domain != "alias.test" {
		t.Fatalf("focused read = %+v, %v; want only alias.test", focused, err)
	}
	if none, _, err := e.store.DomainQuestionsForOwner(ctx, "", 50, "nothing-asked.test"); err != nil || len(none) != 0 {
		t.Fatalf("a domain nobody asked about = %+v, %v; want an empty page", none, err)
	}
}

func TestASoleNameTwinIsSuggestedUntilTheQuestionSettles(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.resolveSiteVerdict(ctx, t, "alice@twin-home.test", "twin-home.test", "Twin Home GmbH")
	e.openTriageFirst(ctx, t, "bob@twin-alias.test", "Bob Brand", "twin-alias.test")
	readID := e.startTriageRead(ctx, t, "twin-alias.test")
	if held := e.answerSiteVerdict(ctx, t, "twin-alias.test", "Twin Home GmbH", readID); held.CompanyID != nil {
		t.Fatal("a name twin linked the domain without a reader")
	}

	got := e.questionAbout(ctx, t, "twin-alias.test").Context.Suggestion
	if got == nil || got.CompanyID != first.CompanyID.UUID || got.Basis != SuggestionNameTwin {
		t.Fatalf("suggestion = %+v, want the twin %s", got, first.CompanyID)
	}
	if _, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "twin-alias.test", Status: DomainCompany, Source: DomainSourceHuman, CompanyID: first.CompanyID, ReadID: readID,
	}); err != nil {
		t.Fatal(err)
	}
	if candidate := e.candidateOf(ctx, t, "twin-alias.test"); candidate != nil {
		t.Fatalf("a settled question still names candidate %s", *candidate)
	}
}

func TestSeveralNameTwinsSuggestNone(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, domain := range []string{"pair-one.test", "pair-two.test"} {
		if _, err := e.store.CreateCompany(ctx, CreateCompanyInput{
			DisplayName: "Pair Works GmbH", Source: "manual",
			Domains: []CompanyDomainInput{{Domain: domain, IsPrimary: true}},
		}); err != nil {
			t.Fatalf("seeding the twin on %s: %v", domain, err)
		}
	}
	e.openTriageFirst(ctx, t, "dana@pair-three.test", "Dana Dorn", "pair-three.test")
	e.answerSiteVerdict(ctx, t, "pair-three.test", "Pair Works GmbH", e.startTriageRead(ctx, t, "pair-three.test"))

	if candidate := e.candidateOf(ctx, t, "pair-three.test"); candidate != nil {
		t.Fatalf("two twins stored candidate %s, want none", *candidate)
	}
	if got := e.questionAbout(ctx, t, "pair-three.test").Context.Suggestion; got != nil {
		t.Fatalf("suggestion = %+v, want none between two twins", got)
	}
}
