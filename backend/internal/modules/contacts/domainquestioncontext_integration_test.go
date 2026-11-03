// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What an open company question tells its reader: whose mail raised it, and
// which existing company, if any, the evidence points at. Every name passes
// the reader's own row scope.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// questionAbout is the reader's open question about one domain.
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
	t.Fatalf("no open question about %s in %+v", domain, rows)
	return DomainQuestion{}
}

// asOwnScope is the rep at the narrowest row scope, who sees only their own
// and workspace-visible records.
func (e *dedupeEnv) asOwnScope() context.Context {
	ctx := e.as()
	p, _ := principal.Actor(ctx)
	p.Permissions.RowScope = principal.RowScopeOwn
	return principal.WithActor(ctx, p)
}

// makePrivateToColleague hands a record to the colleague, visible to them alone.
func (e *dedupeEnv) makePrivateToColleague(ctx context.Context, t *testing.T, table string, id ids.UUID) {
	t.Helper()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET owner_id = $2, visibility = 'owner' WHERE id = $1`, id, e.otherRep)
		return err
	}); err != nil {
		t.Fatalf("making %s %s private to the colleague: %v", table, id, err)
	}
}

func (e *dedupeEnv) employCurrently(ctx context.Context, t *testing.T, contact ids.ContactID, company ids.CompanyID) {
	t.Helper()
	if err := e.store.ResolveContactEmployer(ctx, contact, EmployerChoice{CompanyID: &company, Source: "manual"}); err != nil {
		t.Fatalf("employing %s at %s: %v", contact, company, err)
	}
}

func TestAQuestionNamesItsSenderAndSaysWhetherTheSiteWasRead(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	anna := e.openTriageFirst(ctx, t, "anna@unread.test", "Anna Example", "unread.test")
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "unread.test", SiteUnreadable: true}); err != nil {
		t.Fatal(err)
	}
	got := e.questionAbout(ctx, t, "unread.test").Context
	if len(got.Contacts) != 1 || got.Contacts[0].ID != anna.ContactID || got.Contacts[0].Name != "Anna Example" {
		t.Fatalf("contacts = %+v, want Anna", got.Contacts)
	}
	if got.ContactCount != 1 || got.HiddenSenders || got.OriginID == nil || got.SenderAddress != "anna@unread.test" {
		t.Fatalf("context = %+v, want one visible sender and the mail that raised it", got)
	}
	if got.SiteRead != QuestionSiteUnread {
		t.Fatalf("site_read = %q after no read succeeded, want %q", got.SiteRead, QuestionSiteUnread)
	}
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "unread.test"}); err != nil {
		t.Fatal(err)
	}
	if got := e.questionAbout(ctx, t, "unread.test").Context.SiteRead; got != QuestionSiteRead {
		t.Fatalf("site_read = %q after a read that named nobody, want %q", got, QuestionSiteRead)
	}
}

func TestASenderTheReaderMayNotSeeIsCountedButNotNamed(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	hidden := e.openTriageFirst(ctx, t, "hidden@private.test", "Hidden Sender", "private.test")
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "private.test"}); err != nil {
		t.Fatal(err)
	}
	company := e.createNamedCompany(ctx, t, "Where Hidden Works")
	e.employCurrently(ctx, t, hidden.ContactID, company)
	e.makePrivateToColleague(ctx, t, "contact", hidden.ContactID.UUID)

	got := e.questionAbout(e.asOwnScope(), t, "private.test").Context
	if len(got.Contacts) != 0 || got.ContactCount != 1 || !got.HiddenSenders {
		t.Fatalf("context = %+v, want the private sender counted and not named", got)
	}
	if got.Suggestion != nil {
		t.Fatalf("suggestion = %+v, want none: it would say where a hidden contact works", got.Suggestion)
	}
}

func TestASuggestionNeedsExactlyOneVisibleCompany(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.openTriageFirst(ctx, t, "first@employed.test", "First Sender", "employed.test")
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "employed.test"}); err != nil {
		t.Fatal(err)
	}
	employer := e.createNamedCompany(ctx, t, "Example Employer")
	e.employCurrently(ctx, t, first.ContactID, employer)
	got := e.questionAbout(ctx, t, "employed.test").Context.Suggestion
	if got == nil || got.CompanyID != employer || got.Basis != SuggestionContactEmployer || got.Name != "Example Employer" {
		t.Fatalf("suggestion = %+v, want the sender's employer", got)
	}

	second := e.openTriage(ctx, t, "second@employed.test", "Second Sender", "employed.test")
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "employed.test"}); err != nil {
		t.Fatal(err)
	}
	e.employCurrently(ctx, t, second.ContactID, e.createNamedCompany(ctx, t, "Other Employer"))
	if got := e.questionAbout(ctx, t, "employed.test").Context.Suggestion; got != nil {
		t.Fatalf("suggestion = %+v, want none when two employers qualify", got)
	}

	e.makePrivateToColleague(ctx, t, "company", employer.UUID)
	got = e.questionAbout(e.asOwnScope(), t, "employed.test").Context.Suggestion
	if got == nil || got.Name != "Other Employer" {
		t.Fatalf("suggestion = %+v, want the one employer the reader may still see", got)
	}
}

func TestARedirectTargetSuggestsItsOwnerButDoesNotDecide(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "alex@alias.test", "Alex Example", "alias.test")
	owner := e.createNamedCompany(ctx, t, "Example Company", "target.test")
	if err := e.store.WithholdForwardedDomain(ctx, DomainForwardEvidence{
		RequestedDomain: "alias.test", EffectiveURL: "https://www.target.test/",
		RedirectChain: []string{"https://alias.test", "https://www.target.test/"},
	}); err != nil {
		t.Fatal(err)
	}
	got := e.questionAbout(ctx, t, "alias.test").Context.Suggestion
	if got == nil || got.CompanyID != owner || got.Basis != SuggestionRedirectTarget {
		t.Fatalf("suggestion = %+v, want the company owning the redirect target", got)
	}
	_, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{Domain: "alias.test", Status: DomainCompany, Source: DomainSourceHuman})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("keeping a redirected domain without a company = %v, want a conflict", err)
	}
}

func TestANameTwinIsStoredButTwoTwinsSuggestNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	twin := e.createNamedCompany(ctx, t, "Twin GmbH", "twin-one.test")
	e.createNamedCompany(ctx, t, "Twin GmbH", "twin-two.test")
	e.openTriageFirst(ctx, t, "tom@twin-three.test", "Tom Twin", "twin-three.test")
	e.answerSiteVerdict(ctx, t, "twin-three.test", "Twin GmbH", e.startTriageRead(ctx, t, "twin-three.test"))
	stored := e.candidateOf(ctx, t, "twin-three.test")
	if stored == nil {
		t.Fatal("the held name twin left no candidate on the question")
	}
	got := e.questionAbout(ctx, t, "twin-three.test")
	if got.ReasonCode != PendingNearDuplicate {
		t.Fatalf("reason = %q, want %q", got.ReasonCode, PendingNearDuplicate)
	}
	if got.Context.Suggestion != nil {
		t.Fatalf("suggestion = %+v, want none: two companies carry the name", got.Context.Suggestion)
	}
	if _, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "twin-three.test", Status: DomainCompany, Source: DomainSourceHuman, CompanyID: &twin,
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.candidateOf(ctx, t, "twin-three.test"); got != nil {
		t.Fatalf("candidate = %s after the question settled, want none", got)
	}
}

func (e *dedupeEnv) candidateOf(ctx context.Context, t *testing.T, domain string) *ids.CompanyID {
	t.Helper()
	var candidate *ids.CompanyID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT candidate_company_id FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&candidate)
	}); err != nil {
		t.Fatalf("reading the candidate of %s: %v", domain, err)
	}
	return candidate
}

func TestTheFocusedReadAnswersOnlyTheNamedQuestion(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	for _, domain := range []string{"focus-one.test", "focus-two.test"} {
		e.openTriageFirst(ctx, t, "sales@"+domain, "Sales", domain)
		if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: domain}); err != nil {
			t.Fatal(err)
		}
	}
	rows, _, err := e.store.DomainQuestionsForOwner(ctx, "", 50, "WWW.Focus-Two.test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Domain != "focus-two.test" {
		t.Fatalf("focused read = %+v, want only focus-two.test", rows)
	}
	if rows, _, err := e.store.DomainQuestionsForOwner(e.asOther(), "", 50, "focus-two.test"); err != nil || len(rows) != 0 {
		t.Fatalf("the colleague's focused read = %+v, %v; want nothing", rows, err)
	}
}

// withheldAgain re-opens the unevidenced question new mail rearmed.
func (e *dedupeEnv) withheldAgain(ctx context.Context, t *testing.T, domain string) {
	t.Helper()
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: domain}); err != nil {
		t.Fatal(err)
	}
}

func TestOneTwinSuggestsItself(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	twin := e.createNamedCompany(ctx, t, "Solo GmbH", "solo-one.test")
	e.openTriageFirst(ctx, t, "sam@solo-two.test", "Sam Solo", "solo-two.test")
	e.inTx(ctx, t, "withholding for a name twin", func(tx pgx.Tx) error {
		return withholdForNearDuplicate(ctx, tx, ResolveDomainTriageInput{Domain: "solo-two.test"},
			CompanyCandidateScore{CompanyID: twin, CandidateValue: "Solo GmbH", IncumbentValue: "Solo GmbH"})
	})
	got := e.questionAbout(ctx, t, "solo-two.test").Context.Suggestion
	if got == nil || got.CompanyID != twin || got.Basis != SuggestionNameTwin {
		t.Fatalf("suggestion = %+v, want the one company with that name", got)
	}
}

func TestEverySenderCountsTowardsTheSuggestionNotOnlyTheNamedThree(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	one, other := e.createNamedCompany(ctx, t, "Employer A"), e.createNamedCompany(ctx, t, "Employer B")
	// Oldest first, so the B sender is the one the display cap leaves out.
	outlier := e.openTriageFirst(ctx, t, "outlier@four.test", "Outlier Sender", "four.test")
	e.employCurrently(ctx, t, outlier.ContactID, other)
	for _, name := range []string{"first", "second", "third"} {
		sender := e.openTriage(ctx, t, name+"@four.test", name+" Sender", "four.test")
		e.employCurrently(ctx, t, sender.ContactID, one)
	}
	e.withheldAgain(ctx, t, "four.test")
	got := e.questionAbout(ctx, t, "four.test").Context
	if len(got.Contacts) != 3 || got.ContactCount != 4 {
		t.Fatalf("context = %+v, want three named of four senders", got)
	}
	if got.Suggestion != nil {
		t.Fatalf("suggestion = %+v, want none: a fourth sender works elsewhere", got.Suggestion)
	}
}

func TestAnEmploymentThatEndedYesterdaySuggestsNothing(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	former := e.openTriageFirst(ctx, t, "former@ended.test", "Former Employee", "ended.test")
	e.withheldAgain(ctx, t, "ended.test")
	e.employCurrently(ctx, t, former.ContactID, e.createNamedCompany(ctx, t, "Past Employer"))
	e.inTx(ctx, t, "ending the employment yesterday", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE relationship SET ended_at = current_date - 1
 WHERE contact_id = $1 AND kind = 'employment'`, former.ContactID)
		return err
	})
	if got := e.questionAbout(ctx, t, "ended.test").Context.Suggestion; got != nil {
		t.Fatalf("suggestion = %+v, want none for an employment that has ended", got)
	}
}

func TestTheLinkedCountIsWhatAConfirmEmploys(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.openTriageFirst(ctx, t, "free@linked.test", "Free Sender", "linked.test")
	taken := e.openTriage(ctx, t, "taken@linked.test", "Taken Sender", "linked.test")
	e.employCurrently(ctx, t, taken.ContactID, e.createNamedCompany(ctx, t, "Elsewhere Ltd"))
	e.withheldAgain(ctx, t, "linked.test")
	got := e.questionAbout(ctx, t, "linked.test").Context
	if got.LinkedContactCount != 1 {
		t.Fatalf("linked contacts = %d, want 1: the employed sender keeps their employer", got.LinkedContactCount)
	}
	target := e.createNamedCompany(ctx, t, "Chosen Company")
	res, err := e.store.ResolveDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: "linked.test", Status: DomainCompany, Source: DomainSourceHuman, CompanyID: &target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.EdgesPlanted != got.LinkedContactCount {
		t.Fatalf("the confirm employed %d contacts, the copy promised %d", res.EdgesPlanted, got.LinkedContactCount)
	}
}

func TestASuppressionClearsTheQuestionsContext(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	twin := e.createNamedCompany(ctx, t, "Refused GmbH")
	e.openTriageFirst(ctx, t, "hello@refused.test", "Refused Sender", "refused.test")
	e.inTx(ctx, t, "withholding for a name twin", func(tx pgx.Tx) error {
		return withholdForNearDuplicate(ctx, tx, ResolveDomainTriageInput{Domain: "refused.test"},
			CompanyCandidateScore{CompanyID: twin, CandidateValue: "Refused GmbH", IncumbentValue: "Refused GmbH"})
	})
	if _, err := e.store.SetDomainAdmission(ctx, "refused.test", DomainSuppressed, "not a customer"); err != nil {
		t.Fatal(err)
	}
	if got := e.candidateOf(ctx, t, "refused.test"); got != nil {
		t.Fatalf("candidate = %s after the refusal, want none", got)
	}
	e.openTriageFirst(ctx, t, "hello@refused-unread.test", "Unread Sender", "refused-unread.test")
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{Domain: "refused-unread.test", SiteUnreadable: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.SetDomainAdmission(ctx, "refused-unread.test", DomainSuppressed, "not a customer"); err != nil {
		t.Fatal(err)
	}
	var unreadable bool
	e.inTx(ctx, t, "reading the refused row", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT site_unreadable FROM company_domain_disposition WHERE domain = 'refused-unread.test'`).Scan(&unreadable)
	})
	if unreadable {
		t.Fatal("a refused domain still says its site was never read")
	}
}
