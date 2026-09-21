// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// One exact lane that names TWO contacts, over a real migrated Postgres.
//
// A candidate carrying two addresses that belong to two contacts resolved to
// whichever had the lower id, as a single exact hit at confidence 1, with
// nothing anywhere saying the choice was arbitrary. Routing is unchanged —
// that is the first assertion here, because it is what protects capture — and
// the discarded owner now reaches the review queue.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedTwoOwnersOfOneCard seeds two contacts on two addresses, so a card
// stating both names two of them. Deliberately unalike names on unalike
// domains: this is about ONE lane disagreeing with itself, and a name or
// employer similarity would put a second row in the queue these tests count.
func (e *dedupeEnv) seedTwoOwnersOfOneCard(ctx context.Context, t *testing.T) (lower, higher ids.ContactID) {
	t.Helper()
	first := e.seedContact(ctx, t, "Anna Brandt", []string{"anna@lanesplit-a.test"}, nil)
	second := e.seedContact(ctx, t, "Bernd Kowalski", []string{"bernd@lanesplit-b.test"}, nil)
	if second.String() < first.String() {
		return second, first
	}
	return first, second
}

// THE ROUTING ASSERTION, and it comes first on purpose: capture has to land a
// message somewhere, and it has to be the same somewhere it landed before. The
// lowest id wins, which is what the `ORDER BY … LIMIT 1` this replaced returned.
func TestACardNamingTwoContactsRoutesToTheSameOneItAlwaysDid(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lower, _ := e.seedTwoOwnersOfOneCard(ctx, t)

	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Anna Brandt",
		Emails:   []string{"anna@lanesplit-a.test", "bernd@lanesplit-b.test"},
	})

	if r.Decision != DecisionExactCollision {
		t.Fatalf("decision = %v, want an exact collision — both addresses are live keys", r.Decision)
	}
	if r.ContactID != lower {
		t.Fatalf("routed to %s, want %s: routing is the lowest id and must not move", r.ContactID, lower)
	}
	if r.MatchedLane != LaneEmail {
		t.Errorf("matched lane = %q, want %q", r.MatchedLane, LaneEmail)
	}
}

// And the fact that used to be discarded: the OTHER contact the card named.
func TestACardNamingTwoContactsReportsTheOneItDidNotRouteTo(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lower, higher := e.seedTwoOwnersOfOneCard(ctx, t)

	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Anna Brandt",
		Emails:   []string{"anna@lanesplit-a.test", "bernd@lanesplit-b.test"},
	})

	if r.Conflict == nil {
		t.Fatal("one lane named two contacts and reported no conflict; the losing owner is discarded again")
	}
	if !r.Conflict.SplitWithinLane() {
		t.Errorf("conflict lanes are %q and %q, want one lane disagreeing with itself",
			r.Conflict.RoutedLane, r.Conflict.RivalLane)
	}
	if r.Conflict.RoutedTo != lower || r.Conflict.Rival != higher {
		t.Errorf("conflict names %s over %s, want %s over %s",
			r.Conflict.RoutedTo, r.Conflict.Rival, lower, higher)
	}
}

// A lane that names one contact is not a split, and this is what fails if the
// report ever fires on the ordinary case — a warning on every answer says as
// little as one on none.
func TestACardNamingOneContactReportsNoSplit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	only := e.seedContact(ctx, t, "Clara Vogt", []string{"clara@lanesplit-c.test", "c.vogt@lanesplit-c.test"}, nil)

	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Clara Vogt",
		Emails:   []string{"clara@lanesplit-c.test", "c.vogt@lanesplit-c.test"},
	})

	if r.ContactID != only {
		t.Fatalf("routed to %s, want the one contact both addresses belong to (%s)", r.ContactID, only)
	}
	if r.Conflict != nil {
		t.Errorf("two addresses on ONE contact reported a conflict (%+v); both keys agree", *r.Conflict)
	}
}

// The review row, with the evidence that tells a reader which of the two
// questions this is: the payload disagreed with itself, not our bindings.
func TestALaneSplitEnqueuesOneReviewNamingBothContacts(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	lower, higher := e.seedTwoOwnersOfOneCard(ctx, t)

	r := e.dedupeInTx(ctx, t, ContactCandidate{
		FullName: "Anna Brandt",
		Emails:   []string{"anna@lanesplit-a.test", "bernd@lanesplit-b.test"},
	})
	if r.Conflict == nil {
		t.Fatal("no conflict to enqueue")
	}
	recorded, err := e.store.EnqueueIdentityConflict(ctx, *r.Conflict, "vcard_import", "human:test")
	if err != nil {
		t.Fatalf("EnqueueIdentityConflict: %v", err)
	}
	if !recorded {
		t.Fatal("the first split on this pair must record a new row")
	}

	rows := openCandidates(ctx, t, e, entityContact)
	if len(rows) != 1 {
		t.Fatalf("open queue holds %d candidates, want exactly 1", len(rows))
	}
	pair := map[ids.UUID]bool{rows[0].LeftID: true, rows[0].RightID: true}
	if !pair[lower.UUID] || !pair[higher.UUID] {
		t.Fatalf("candidate pair = {%s, %s}, want {%s, %s}", rows[0].LeftID, rows[0].RightID, lower, higher)
	}

	var evidence []evidenceEntry
	if err := json.Unmarshal(rows[0].Evidence, &evidence); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	split := false
	for _, ev := range evidence {
		if ev.Signal == evidenceSignalLaneSplit && ev.LeftValue != nil && *ev.LeftValue == LaneEmail {
			split = true
		}
	}
	if !split {
		t.Errorf("evidence %+v carries no %s row naming the email lane — a reader cannot tell this "+
			"from two bindings disagreeing", evidence, evidenceSignalLaneSplit)
	}
}

// The company half: two of a candidate's domains registered to two companies.
//
// The company ladder has ONE exact lane, so this can never be two lanes
// disagreeing — it is always the payload disagreeing with itself. The employer
// inference rides this lane, so the cost of discarding the loser is a contact
// attached to whichever company had the lower id, with nothing saying a second
// one answered to the same sender.
func TestDomainsNamingTwoCompaniesRouteToTheSameOneAndReportTheOther(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first := e.seedCompanyOnDomain(ctx, t, "Kellner Anlagenbau", "kellner-split.test")
	second := e.seedCompanyOnDomain(ctx, t, "Kellner Services", "kellner-services-split.test")
	lower, higher := first, second
	if second.String() < first.String() {
		lower, higher = second, first
	}

	var match CompanyMatch
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		match, err = DedupeCompany(ctx, tx, CompanyCandidate{
			Domains: []string{"kellner-split.test", "kellner-services-split.test"},
		})
		return err
	}); err != nil {
		t.Fatalf("DedupeCompany: %v", err)
	}

	if match.Decision != DecisionExactCollision {
		t.Fatalf("decision = %v, want an exact collision — both domains are live keys", match.Decision)
	}
	if match.CompanyID != lower {
		t.Fatalf("routed to %s, want %s: routing is the lowest id and must not move", match.CompanyID, lower)
	}
	if match.DomainSplit == nil {
		t.Fatal("two domains named two companies and reported no split; the losing one is discarded again")
	}
	if match.DomainSplit.RoutedTo != lower || match.DomainSplit.Rival != higher {
		t.Errorf("split names %s over %s, want %s over %s",
			match.DomainSplit.RoutedTo, match.DomainSplit.Rival, lower, higher)
	}

	recorded, err := e.store.EnqueueDomainSplit(ctx, *match.DomainSplit, "capture:mail", "connector:gmail")
	if err != nil {
		t.Fatalf("EnqueueDomainSplit: %v", err)
	}
	if !recorded {
		t.Fatal("the first split on this pair must record a new row")
	}
	rows := openCandidates(ctx, t, e, entityCompany)
	if len(rows) != 1 {
		t.Fatalf("open company queue holds %d candidates, want exactly 1", len(rows))
	}
	pair := map[ids.UUID]bool{rows[0].LeftID: true, rows[0].RightID: true}
	if !pair[lower.UUID] || !pair[higher.UUID] {
		t.Fatalf("candidate pair = {%s, %s}, want {%s, %s}", rows[0].LeftID, rows[0].RightID, lower, higher)
	}
}

// Two domains on ONE company are not a split, which is the ordinary case: a
// sender's own domain and its base usually belong to the same account, and a
// report that fired there would be a report on every message.
func TestTwoDomainsOnOneCompanyReportNoSplit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	only := e.seedCompanyOnDomain(ctx, t, "Turbinenbau AG", "turbinenbau-split.test")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO company_domain (company_id, domain, is_primary, source, captured_by)
			VALUES ($1, 'mail.turbinenbau-split.test', false, 'manual', 'human:test')`, only)
		return err
	}); err != nil {
		t.Fatalf("seeding the second domain: %v", err)
	}

	var match CompanyMatch
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		var err error
		match, err = DedupeCompany(ctx, tx, CompanyCandidate{
			Domains: []string{"mail.turbinenbau-split.test", "turbinenbau-split.test"},
		})
		return err
	}); err != nil {
		t.Fatalf("DedupeCompany: %v", err)
	}
	if match.CompanyID != only {
		t.Fatalf("routed to %s, want the one company both domains belong to (%s)", match.CompanyID, only)
	}
	if match.DomainSplit != nil {
		t.Errorf("two domains on ONE company reported a split (%+v); both keys agree", *match.DomainSplit)
	}
}
