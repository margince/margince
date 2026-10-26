// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What the OPERATOR's list carries, over a real Postgres.
//
// The list answers one question nothing else can: a company that never appeared,
// was it refused, and by whom? Its rows are therefore every decision and
// every open question. An open question also reaches its decider's own queue
// (domainquestionlist.go): the sole direct recipient of current mail.
//
// These live over a database rather than behind a fake because the whole subject
// is one WHERE clause, and nothing but SQL decides which rows clear it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// withholdUnreadable opens a domain's question and leaves it unanswered, which
// is the state both tests below are about.
func (e *dedupeEnv) withholdUnreadable(ctx context.Context, t *testing.T, sender, domain string) {
	t.Helper()
	e.openTriage(ctx, t, sender, "", domain)
	if _, err := e.store.ResolveUnreadableDomainTriage(ctx, ResolveDomainTriageInput{
		Domain: domain, SeedURL: TriageSeedURL(domain),
		Evidence: "the site could not be read",
	}); err != nil {
		t.Fatalf("resolve unreadable for %s: %v", domain, err)
	}
}

// listedDomains is what the operator's list carries, as domains.
func (e *dedupeEnv) listedDomains(ctx context.Context, t *testing.T) []string {
	t.Helper()
	entries, _, err := e.store.ListDomainAdmissions(ctx, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Domain)
	}
	return out
}

// An ownerless open question reaches the operator's list. No personal queue
// may carry it, and without this list it would be a company held out of the
// CRM that no screen admits exists.
func TestTheDomainListCarriesTheQuestionsNobodyOwns(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.withholdUnreadable(ctx, t, "hello@pwc.example", "pwc.example")
	e.releaseQuestion(ctx, t, "pwc.example")

	entries, total, err := e.store.ListDomainAdmissions(ctx, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("list = %d entries (total %d), want the one unowned question", len(entries), total)
	}
	got := entries[0]
	if got.Domain != "pwc.example" {
		t.Fatalf("domain = %q, want the withheld one", got.Domain)
	}
	if got.Admission != DomainUndecided {
		t.Errorf("admission = %q, want %q — nobody decided this domain", got.Admission, DomainUndecided)
	}
	// The source says what STOPPED the machine, which is the only thing an
	// operator can act on: a site that named nothing is a different problem
	// from mail too old to trust.
	if got.Source != PendingUnevidenced {
		t.Errorf("source = %q, want %q", got.Source, PendingUnevidenced)
	}
	if got.Reason == "" {
		t.Error("the open question carries no sentence — a row nobody can read is one nobody can answer")
	}
	// Dated from the last time the row moved. An undecided row has no
	// admission_at, and serving a zero time would sort every open question
	// under the epoch.
	if got.DecidedAt.IsZero() {
		t.Error("decided_at is zero — an undecided row still has to say when it last moved")
	}
}

// A deactivated owner's open question stays on this list. Deactivation takes
// away the personal queue that was also carrying it.
//
// This is the case the column alone cannot see. Deactivating a seat keeps the
// app_user row and revokes its sessions, so owner_id still names somebody and
// that somebody can no longer sign in. The per-owner queue matches `owner_id =
// $1` for the calling human, so the question is served to nobody: not to the
// departed owner, who cannot authenticate, and not to any colleague. Nothing
// re-asks either — the withholding cleared the retry cursor.
func TestADeactivatedOwnersQuestionReachesTheOperatorsList(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	// Owned by e.rep, which is what the capture path stamps: the disposition
	// records the mailbox owner the message reached, and the fixture's ensure
	// input names that seat.
	e.withholdUnreadable(ctx, t, "hello@departing.example", "departing.example")
	if listed := e.listedDomains(ctx, t); !contains(listed, "departing.example") {
		t.Fatalf("the operator's list = %v, want departing.example listed while its owner "+
			"can still answer it, as every open question is", listed)
	}

	// They leave. Deactivation is identity's act; what it writes to app_user is
	// spelled here because contacts may not import that module to call it. The
	// read below still runs as that seat — ListDomainAdmissions asks RBAC, not
	// whether the caller could sign in today — so what changes is the question's
	// reachability, not the reader's.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, e.rep)
		return err
	}); err != nil {
		t.Fatalf("deactivating the owner: %v", err)
	}

	if listed := e.listedDomains(ctx, t); !contains(listed, "departing.example") {
		t.Errorf("the operator's list = %v, and departing.example is on none of them: its "+
			"owner cannot sign in to answer it and no colleague is shown it, so the "+
			"question is a company held out of the CRM that nothing admits exists", listed)
	}
}

// An open question stays on this list even while a live seat owns it.
//
// Every pending reason has one decider, the sole direct recipient of current
// mail (openQuestionForReaderSQL). When there is none, nobody is asked, and
// this list is where the fact stays. A vendor question was always listed for
// that reason. Every reason now is, so an operator sees one list of what is
// held out of the CRM. A colleague reads the same list as the owner.
func TestAnOwnedQuestionIsAlsoOnTheOperatorsList(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.withholdUnreadable(ctx, t, "hello@mine.example", "mine.example")

	if listed := e.listedDomains(ctx, t); !contains(listed, "mine.example") {
		t.Errorf("the owner's list = %v, want the open question listed", listed)
	}
	if listed := e.listedDomains(e.asOther(), t); !contains(listed, "mine.example") {
		t.Errorf("the colleague's list = %v, want the same open question", listed)
	}
}
