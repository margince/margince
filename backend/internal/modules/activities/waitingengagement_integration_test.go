// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The two rules that decide whether a waiting message is a CUSTOMER waiting:
// who wrote it, and whether this workspace was ever in the conversation.
//
// They differ in what they are allowed to do, and the difference is the point.
// A colleague's domain is a fact about us — an administrator vouched for it — so
// it EXCLUDES. Prior engagement is read from thread identity, which comes from
// headers the sender chose to send, so it only DEMOTES: a client that strips
// References gives every message its own thread, and excluding on that would
// drop a live customer with nothing on the page to say so.
//
// Both need an admit case beside every refuse case. Three security tests in this
// tree once passed against an authority that refused everyone, and a rule that
// hides everything hides the real work too.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fixedDomains is the own-domain seam under test, answering from a literal.
//
// A double here rather than capture's real store because what is under test is
// the QUERY's use of the list, not how the list is assembled — capture's own
// tests hold that. The seam takes the caller's transaction, and this honours it
// by ignoring it: there is nothing to read.
type fixedDomains []string

func (f fixedDomains) Domains(context.Context, pgx.Tx) ([]string, error) {
	return []string(f), nil
}

// storeKnowing builds the queue store with a colleague-domain answer bound.
func storeKnowing(e *loadEnv, domains ...string) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).
		WithOwnDomains(fixedDomains(domains))
}

// waitingFrom seeds one otherwise-qualifying wait from a named address.
func (e *loadEnv) waitingFrom(t *testing.T, subject, address string, person ids.UUID) ids.UUID {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'inbound', $2, now() - interval '2 days', $3, 'seed', 'system')`,
		activity, subject, "thread-"+activity.String())
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
		VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, address)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, person_id)
		VALUES ($1, $2, 'person', $3)`, ids.NewV7(), activity, person)
	return activity
}

// buyer writes one person record for a wait to hang off.
func (e *loadEnv) buyer(t *testing.T) ids.UUID {
	t.Helper()
	person := ids.NewV7()
	e.exec(t, `INSERT INTO person (id, full_name, owner_id, source, captured_by)
		VALUES ($1, 'Buyer Person', $2, 'seed', 'system')`, person, e.rep)
	return person
}

// present reports whether the given message came back from the queue.
func present(t *testing.T, s *Store, e *loadEnv, activity ids.UUID) (WaitingReply, bool) {
	t.Helper()
	rows, err := s.WaitingReplies(e.as(), time.Now())
	if err != nil {
		t.Fatalf("reading who is waiting: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == activity {
			return row, true
		}
	}
	return WaitingReply{}, false
}

// A colleague writing is not a customer waiting.
func TestAMessageFromOurOwnDomainIsNotAWaitingCustomer(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	colleague := e.waitingFrom(t, "Outstanding invoices", "eric@ourco.test", person)
	customer := e.waitingFrom(t, "Question about pricing", "buyer@customer.test", person)

	s := storeKnowing(e, "ourco.test")
	if _, ok := present(t, s, e, colleague); ok {
		t.Error("a colleague's message is in the queue as a waiting customer")
	}
	// The admit case, and it is not decoration: a rule that refused every
	// sender would pass the assertion above and empty the queue.
	if _, ok := present(t, s, e, customer); !ok {
		t.Error("the customer's message was excluded too — the rule refuses everyone")
	}
}

// A departmental host is still our own house.
func TestASubdomainOfOursIsAlsoOurs(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	sub := e.waitingFrom(t, "From the Berlin office", "ops@mail.ourco.test", person)
	// A domain that merely ENDS with ours is somebody else's company.
	lookalike := e.waitingFrom(t, "Not us at all", "sales@notourco.test", person)

	s := storeKnowing(e, "ourco.test")
	if _, ok := present(t, s, e, sub); ok {
		t.Error("mail from a subdomain of ours was treated as a customer")
	}
	if _, ok := present(t, s, e, lookalike); !ok {
		t.Error("a different company whose domain ends with ours was excluded")
	}
}

// A sender cannot get their own message suppressed by naming us inside it.
//
// A quoted local part may legally contain an at-sign, so `"x@ourco.test"@evil.test`
// is one address whose domain is evil.test. Reading the domain after the FIRST
// at-sign returns `ourco.test"` — one trailing quote from a match — and a sender
// who trimmed it right would have their message silently dropped from every
// rep's queue. The domain is read after the LAST at-sign for that reason.
func TestASenderCannotHideBehindOurDomainInTheirLocalPart(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	quoted := e.waitingFrom(t, "Suppress me", `"x@ourco.test"@evil.test`, person)
	doubled := e.waitingFrom(t, "And me", "a@ourco.test@evil.test", person)

	s := storeKnowing(e, "ourco.test")
	if _, ok := present(t, s, e, quoted); !ok {
		t.Error("a quoted local part naming our domain suppressed the sender's message")
	}
	if _, ok := present(t, s, e, doubled); !ok {
		t.Error("an address with two at-signs was read as internal")
	}
}

// An own-domain entry is operator-typed text, never a pattern.
//
// Underscore and percent are LIKE wildcards. An entry of `our_.test` matched
// `ourx.test` while this rule used LIKE, which hides a real customer's mail
// across the whole workspace with nothing on any page to say why. The suffix is
// compared with right() so an entry can only ever match itself.
func TestAnOwnDomainIsNeverReadAsAPattern(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	// A SUBDOMAIN, because that is the branch a pattern can reach: the equality
	// branch compares whole strings and can never treat one as a pattern, so a
	// fixture using a bare domain passes whether or not the suffix is safe.
	customer := e.waitingFrom(t, "A real customer", "buyer@mail.ourx.test", person)

	// The operator typed a wildcard character, deliberately or by accident.
	s := storeKnowing(e, "our_.test")
	if _, ok := present(t, s, e, customer); !ok {
		t.Error("an underscore in an own-domain entry matched a customer's subdomain")
	}
}

// An empty entry matches nothing, not everything.
//
// The suffix comparison would otherwise be against a bare dot, and an empty
// string equals no domain — but a blank row in the list must not be able to
// empty the queue, so it is refused explicitly.
func TestABlankOwnDomainMatchesNothing(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	// A subdomain again: a blank entry makes the suffix comparison ask whether
	// the domain ends in a bare dot, which only a dotted address could satisfy.
	// Against "customer.test" the test would pass with the guard removed.
	customer := e.waitingFrom(t, "A real customer", "buyer@mail.customer.test", person)

	s := storeKnowing(e, "")
	if _, ok := present(t, s, e, customer); !ok {
		t.Error("a blank own-domain entry excluded a customer")
	}
}

// No domains wired, nobody excluded.
//
// The open default is deliberate: a deployment that cannot say which domains are
// its own must not guess. A queue with colleagues in it is visible; a queue that
// dropped a customer whose domain resembles ours is not.
func TestWithNoOwnDomainsNobodyIsExcluded(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	colleague := e.waitingFrom(t, "Outstanding invoices", "eric@ourco.test", person)

	unbound := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	if _, ok := present(t, unbound, e, colleague); !ok {
		t.Error("an unwired store excluded a sender it has no domain list to judge")
	}
}

// Prior engagement is REPORTED, and a message without it still comes back.
//
// This is the whole shape of the rule. If it ever becomes an exclusion, this
// test fails — which is what it is for.
func TestAnUnengagedThreadIsReportedNotHidden(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	cold := e.waitingFrom(t, "Cold approach", "stranger@customer.test", person)

	s := storeKnowing(e, "ourco.test")
	row, ok := present(t, s, e, cold)
	if !ok {
		t.Fatal("a thread we never wrote on was HIDDEN — it must only be demoted")
	}
	if row.Engaged {
		t.Error("a thread with no earlier outbound reported itself as engaged")
	}
}

// An earlier outbound on the same thread is what engagement means.
func TestAnEarlierOutboundOnTheThreadIsEngagement(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Re: the proposal", "buyer@customer.test", person)
	var thread string
	e.exec(t, `SELECT 1`)
	if err := e.pool.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`,
		activity).Scan(&thread); err != nil {
		t.Fatalf("reading the seeded thread key: %v", err)
	}
	// Ours, on the same thread, BEFORE the inbound arrived.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'the proposal', now() - interval '5 days', $2, 'seed', 'system')`,
		ids.NewV7(), thread)

	s := storeKnowing(e, "ourco.test")
	row, ok := present(t, s, e, activity)
	if !ok {
		t.Fatal("an engaged thread left the queue entirely")
	}
	if !row.Engaged {
		t.Error("an earlier outbound on the same thread did not count as engagement")
	}
}

// Engagement is what we wrote BEFORE, not after.
//
// Reading engagement without the ordering would call a thread engaged on the
// strength of a reply that has not been sent yet — which is the one thing that
// must not happen, because a scheduled send would then take the customer out of
// the band before anybody answered them.
func TestALaterOutboundIsNotEngagement(t *testing.T) {
	e := setupLoad(t)
	person := e.buyer(t)
	activity := e.waitingFrom(t, "Still waiting", "buyer@customer.test", person)
	var thread string
	if err := e.pool.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`,
		activity).Scan(&thread); err != nil {
		t.Fatalf("reading the seeded thread key: %v", err)
	}
	// Ours, on the same thread, dated in the FUTURE. That is what isolates the
	// ordering guard: the reply anti-join beside it is bounded by the read
	// instant, so this row does not remove the wait — and if engagement ignored
	// the ordering, this row alone would make the thread read as engaged.
	//
	// A scheduled send is exactly this shape, so the case is real rather than
	// contrived: a queued reply must not make a customer look answered.
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'queued for tomorrow', now() + interval '1 day', $2, 'seed', 'system')`,
		ids.NewV7(), thread)

	s := storeKnowing(e, "ourco.test")
	row, ok := present(t, s, e, activity)
	if !ok {
		t.Fatal("the wait left the queue")
	}
	if row.Engaged {
		t.Error("a not-yet-sent outbound counted as engagement")
	}
}
