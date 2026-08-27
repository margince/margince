// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"testing"
	"time"
)

// refsWithProjects builds the minimum pipelineRefs projectForActivity reads:
// a fixed "now" so date offsets are stable, and one company's projects in the
// oldest-first order loadProjects guarantees.
func refsWithProjects(projects ...seededProject) pipelineRefs {
	return pipelineRefs{
		now: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		// Two domains onto one account, because that is the shape the
		// by-domain keying used to lose: whichever alias the reversal
		// happened to keep was the only one that resolved.
		orgsByDom:     map[string]string{"acme.test": "org-acme", "acme.example": "org-acme"},
		projectsByOrg: map[string][]seededProject{"org-acme": projects},
	}
}

// TestActivityPicksTheProjectThatHadStarted — the defect this rule exists for.
//
// valantic has two projects: the Shopsystem migration from October, and a
// second tenant that starts in August the following year. All nine of the
// account's mails are about the migration. Taking whichever project the API
// listed first put every one of them on the second tenant — onto a project
// that begins nine months after the last of those mails was sent, which is a
// timeline that could not have happened.
func TestActivityLinksToTheProjectThatHadAlreadyStarted(t *testing.T) {
	refs := refsWithProjects(
		seededProject{ID: "migration", StartedAt: "2025-10-28"},
		seededProject{ID: "second-tenant", StartedAt: "2026-08-06"},
	)

	// A mail from 214 days ago — after the migration began, long before the
	// second tenant did.
	got := projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 214})
	if got != "migration" {
		t.Errorf("a mail sent during the migration linked to %q, want migration", got)
	}

	// A mail from last week belongs to the newer work, which had started by then.
	got = projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 7})
	if got != "second-tenant" {
		t.Errorf("a mail sent after the second tenant began linked to %q, want second-tenant", got)
	}
}

// TestActivityOlderThanEveryProjectLinksToNone — correspondence from before
// any delivery work began was not about delivery work, and saying it was is a
// worse answer than saying nothing.
func TestActivityOlderThanEveryProjectLinksToNone(t *testing.T) {
	refs := refsWithProjects(seededProject{ID: "migration", StartedAt: "2025-10-28"})

	if got := projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 900}); got != "" {
		t.Errorf("a mail predating every project linked to %q, want no link at all", got)
	}
}

// TestActivityOnACompanyWithNoProjectLinksToNone — the ~180 crawled companies
// the planner gives no project to, which must not panic on an absent map key.
func TestActivityOnACompanyWithNoProjectLinksToNone(t *testing.T) {
	refs := refsWithProjects()
	if got := projectForActivity(refs, demoActivity{Company: "nobody.test", DaysAgo: 30}); got != "" {
		t.Errorf("a company with no project linked to %q, want no link", got)
	}
	if got := projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 30}); got != "" {
		t.Errorf("an empty project list linked to %q, want no link", got)
	}
}

// TestAFutureActivityLinksToWhatHasStarted — a booked meeting or an open task
// carries DaysIn rather than DaysAgo, and is dated forward.
func TestAFutureActivityLinksToWhatHasStarted(t *testing.T) {
	refs := refsWithProjects(
		seededProject{ID: "migration", StartedAt: "2025-10-28"},
		seededProject{ID: "second-tenant", StartedAt: "2026-08-06"},
	)
	if got := projectForActivity(refs, demoActivity{Company: "acme.test", DaysIn: 14}); got != "second-tenant" {
		t.Errorf("a task due in a fortnight linked to %q, want second-tenant", got)
	}
}

// TestAProjectWithNoStartDateIsTheLastResort — started_at is nullable on the
// contract, so a project can carry none. It cannot be dated against, and must
// never displace one that can be.
func TestAProjectWithNoStartDateIsTheLastResort(t *testing.T) {
	refs := refsWithProjects(
		seededProject{ID: "dated", StartedAt: "2025-10-28"},
		seededProject{ID: "undated"},
	)
	if got := projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 30}); got != "dated" {
		t.Errorf("linked to %q, want the project that has a start date", got)
	}
}

// TestProjectRelinkFor covers every state an activity already on file can be
// in when the reconciliation pass reaches it.
//
// Each relink is a write on a surface that stamps a six-year retention class
// the database will not let anyone lift, so "do nothing" is the answer that
// has to be right most often.
func TestProjectRelinkFor(t *testing.T) {
	refs := refsWithProjects(
		seededProject{ID: "migration", StartedAt: "2025-10-28"},
		seededProject{ID: "second-tenant", StartedAt: "2026-08-06"},
	)
	act := demoActivity{Company: "acme.test", DaysAgo: 214}
	// During the migration, long before the second tenant began.
	const during = "2026-01-22T09:00:00Z"

	for _, tc := range []struct {
		name     string
		existing seededActivity
		act      demoActivity
		wantID   string
		wantMove bool
	}{
		{
			// The shape a pre-projects installation is in: the activity
			// exists, carries no project link, and reseeding never gave it
			// one because the create path replays instead of re-linking.
			name:     "unfiled activity is filed",
			existing: seededActivity{ID: "a1", OccurredAt: during},
			act:      act,
			wantID:   "migration",
			wantMove: true,
		},
		{
			// What the old list-order linker left behind: filed under a
			// project that had not started when the mail was sent.
			name:     "wrongly filed activity is moved",
			existing: seededActivity{ID: "a1", OccurredAt: during, ProjectID: "second-tenant"},
			act:      act,
			wantID:   "migration",
			wantMove: true,
		},
		{
			// A converged installation. The pass must be silent.
			name:     "correctly filed activity is left alone",
			existing: seededActivity{ID: "a1", OccurredAt: during, ProjectID: "migration"},
			act:      act,
			wantMove: false,
		},
		{
			// Older than every project on the account, so it is about none of
			// them. Filing it anyway stamps a record that had no reason to be.
			name:     "activity predating every project is left alone",
			existing: seededActivity{ID: "a1", OccurredAt: "2024-01-05T09:00:00Z"},
			act:      act,
			wantMove: false,
		},
		{
			name:     "activity on a company with no project is left alone",
			existing: seededActivity{ID: "a1", OccurredAt: during},
			act:      demoActivity{Company: "nobody.test"},
			wantMove: false,
		},
		{
			// An activity the server returned without a date cannot be dated
			// against, and guessing is what this function exists to avoid.
			name:     "activity with no occurred_at is left alone",
			existing: seededActivity{ID: "a1"},
			act:      act,
			wantMove: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotMove := projectRelinkFor(refs, tc.act, tc.existing)
			if gotMove != tc.wantMove {
				t.Fatalf("move = %v, want %v", gotMove, tc.wantMove)
			}
			if gotMove && gotID != tc.wantID {
				t.Errorf("project = %q, want %q", gotID, tc.wantID)
			}
		})
	}
}

// TestTheReconciliationDatesByTheRecordNotTheDataset — the drift a review
// caught, and the reason projectRelinkFor takes its date from the stored row.
//
// days_ago is relative to the day the seeder RUNS. occurred_at was frozen by
// the first run and never moves again. So on any later day the two disagree,
// and a pass that dated by the offset would decide an activity nothing had
// touched now belongs to a different project — then relink it, stamping
// six-year retention that cannot be lifted.
func TestTheReconciliationDatesByTheRecordNotTheDataset(t *testing.T) {
	refs := refsWithProjects(
		seededProject{ID: "migration", StartedAt: "2025-10-28"},
		seededProject{ID: "second-tenant", StartedAt: "2026-08-06"},
	)
	// The record says this mail was sent during the migration, and that does
	// not change however long ago the seeder last ran.
	existing := seededActivity{ID: "a1", OccurredAt: "2026-01-22T09:00:00Z", ProjectID: "migration"}

	// The same entry read on a day when its days_ago offset would now land
	// after the second tenant started. Dating by the offset would move it.
	act := demoActivity{Company: "acme.test", DaysAgo: 1}
	if _, move := projectRelinkFor(refs, act, existing); move {
		t.Error("the pass dated by the dataset offset and would relink an activity nothing had changed")
	}
	if want := projectForActivity(refs, act); want != "second-tenant" {
		t.Fatalf("the fixture no longer demonstrates the drift: offset dating gives %q", want)
	}
}

// TestASeededActivityIsIdentifiedByBothHalvesOfItsKey — source_id alone is
// not an identity.
//
// The database is unique on (source_system, source_id), and this tool's own
// ids are "act-0", "act-1" — spellings any connector is free to use. While the
// activity index only counted rows, a collision merely miscounted. Now that it
// decides which activity gets RELINKED, the same collision would file a
// stranger's mail under a demo project and stamp it with six-year retention
// that cannot be lifted.
func TestASeededActivityIsIdentifiedByBothHalvesOfItsKey(t *testing.T) {
	page := json.RawMessage(`[
	  {"id":"theirs","source_system":"gmail","source_id":"act-0","occurred_at":"2026-01-22T09:00:00Z",
	   "links":[{"entity_type":"organization","entity_id":"org-2"}]},
	  {"id":"ours","source_system":"seed","source_id":"act-0","occurred_at":"2026-01-22T09:00:00Z",
	   "links":[{"entity_type":"organization","entity_id":"org-1"}]}
	]`)
	seen := map[string]seededActivity{}
	if err := indexSeededActivities(page, seen); err != nil {
		t.Fatalf("indexing the page: %v", err)
	}
	if len(seen) != 1 {
		t.Errorf("indexed %d activities, want only the seeded one", len(seen))
	}
	got, ok := seen["act-0"]
	if !ok {
		t.Fatal("the seeded activity was not indexed at all")
	}
	if got.ID != "ours" {
		t.Errorf("act-0 resolved to %q — a row from another source system claimed the key", got.ID)
	}
	if got.OrganizationID != "org-1" {
		t.Errorf("organization = %q, want org-1", got.OrganizationID)
	}
}

// TestTheIndexReadsBothLinksItActsOn — the reconciliation decides from the
// project link (what is filed now) and the organization link (whether this is
// even the right row), so a page that carries both must yield both.
func TestTheIndexReadsBothLinksItActsOn(t *testing.T) {
	page := json.RawMessage(`[
	  {"id":"a1","source_system":"seed","source_id":"act-7","occurred_at":"2026-01-22T09:00:00Z",
	   "links":[{"entity_type":"organization","entity_id":"org-1"},
	            {"entity_type":"person","entity_id":"p-1"},
	            {"entity_type":"project","entity_id":"proj-1"}]}
	]`)
	seen := map[string]seededActivity{}
	if err := indexSeededActivities(page, seen); err != nil {
		t.Fatalf("indexing the page: %v", err)
	}
	got := seen["act-7"]
	if got.ProjectID != "proj-1" {
		t.Errorf("project = %q, want proj-1", got.ProjectID)
	}
	if got.OrganizationID != "org-1" {
		t.Errorf("organization = %q, want org-1", got.OrganizationID)
	}
	if got.OccurredAt == "" {
		t.Error("occurred_at was dropped; the reconciliation dates against it")
	}
}

// TestEitherDomainOfOneAccountFindsTheSameWork — the defect the organization
// keying exists for.
//
// The projects map was keyed by domain and built by reversing orgsByDom, so an
// account with two domains kept exactly one of them and which one was Go map
// iteration order. An activity naming the other found no projects at all, and
// found them again on the next run. Both of an account's domains name the same
// account, so both must reach the same delivery work.
func TestEitherDomainOfOneAccountFindsTheSameWork(t *testing.T) {
	refs := refsWithProjects(seededProject{ID: "migration", StartedAt: "2026-01-15"})

	named := projectForActivity(refs, demoActivity{Company: "acme.test", DaysAgo: 30})
	alias := projectForActivity(refs, demoActivity{Company: "acme.example", DaysAgo: 30})

	if named != "migration" {
		t.Fatalf("the account's own domain reaches %q, want the migration", named)
	}
	if alias != named {
		t.Errorf("the alias reaches %q and the domain reaches %q; one account, one answer", alias, named)
	}
}
