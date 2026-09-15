// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What the OPERATOR's list carries, over a real Postgres.
//
// The list answers one question nothing else can: a company that never appeared
// — was it refused, and by whom? Its rows are therefore every decision, plus the
// open questions that have nobody to ask. An open question WITH an owner is not
// here: it reaches that colleague's own queue (domainquestionlist.go), which is
// the only surface carrying the verbs that answer it.
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

// An OWNERLESS open question reaches the operator's list, because no other
// surface can carry it.
//
// A question is addressed to the mailbox owner whose mail raised it, and that
// colleague answers it on their own queue. This row has no such owner — the
// column is nullable, and it is cleared outright when the owner's account is
// deleted — so the per-owner read, which matches `owner_id = $1`, matches it for
// nobody. Without this list it would be a company held out of the CRM that no
// screen admits exists.
func TestTheDomainListCarriesTheQuestionsNobodyOwns(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.withholdUnreadable(ctx, t, "hello@pwc.example", "pwc.example")
	// The owner departs. ON DELETE SET NULL is what clears the column in
	// production; the column is set directly here because deleting an app_user
	// row is identity's act and not this package's to perform.
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE company_domain_disposition SET owner_id = NULL WHERE domain = $1`, "pwc.example")
		return err
	}); err != nil {
		t.Fatalf("clearing the owner: %v", err)
	}

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

// A DEACTIVATED owner's open question reaches this list, because deactivation
// takes away the one surface that was carrying it.
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
	if listed := e.listedDomains(ctx, t); contains(listed, "departing.example") {
		t.Fatalf("the operator's list = %v, want departing.example absent while its owner "+
			"can still answer it on their own queue", listed)
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

// An open question owned by a LIVE seat stays off this list, whoever is reading.
//
// It has an addressee who can still answer it, on their own queue, with the two
// verbs that settle it. Carrying it here as well would put one question on two
// surfaces — and offer an operator a re-ask on mail they cannot read, whose
// answer belongs to somebody else.
//
// Both readers are asserted because the clause is about the ROW, not about who
// is looking: it asks whether anybody can still answer the question, so a
// colleague sees exactly what the owner does. The fixture's capture path stamps
// e.rep as the owner of every disposition it opens, so what the second read
// varies is the reader alone, which is the whole point of it.
func TestAnOwnedQuestionStaysOffTheOperatorsList(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.withholdUnreadable(ctx, t, "hello@mine.example", "mine.example")

	if listed := e.listedDomains(ctx, t); len(listed) != 0 {
		t.Errorf("the owner's list = %v, want the owned question absent: it is on their "+
			"queue, and one question on two surfaces is one two colleagues answer "+
			"differently", listed)
	}
	// A colleague reading the same list sees no more than the owner does.
	if listed := e.listedDomains(e.asOther(), t); len(listed) != 0 {
		t.Errorf("the colleague's list = %v, want it empty too: a live seat owns the "+
			"question, so it is not this list's to carry for anyone", listed)
	}
}
