// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"testing"
	"time"
)

// refsWithProjects builds the minimum pipelineRefs projectForActivity reads:
// a fixed "now" so date offsets are stable, and one company's projects in the
// oldest-first order loadProjects guarantees.
func refsWithProjects(projects ...seededProject) pipelineRefs {
	return pipelineRefs{
		now:               time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
		projectsByCompany: map[string][]seededProject{"acme.test": projects},
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
	// A mail from during the migration, which is what the fixtures are about.
	act := demoActivity{Company: "acme.test", DaysAgo: 214}

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
			existing: seededActivity{ID: "a1"},
			act:      act,
			wantID:   "migration",
			wantMove: true,
		},
		{
			// What the old list-order linker left behind: filed under a
			// project that had not started when the mail was sent.
			name:     "wrongly filed activity is moved",
			existing: seededActivity{ID: "a1", ProjectID: "second-tenant"},
			act:      act,
			wantID:   "migration",
			wantMove: true,
		},
		{
			// A converged installation. The pass must be silent.
			name:     "correctly filed activity is left alone",
			existing: seededActivity{ID: "a1", ProjectID: "migration"},
			act:      act,
			wantMove: false,
		},
		{
			// Older than every project on the account, so it is about none of
			// them. Filing it anyway stamps a record that had no reason to be.
			name:     "activity predating every project is left alone",
			existing: seededActivity{ID: "a1"},
			act:      demoActivity{Company: "acme.test", DaysAgo: 900},
			wantMove: false,
		},
		{
			name:     "activity on a company with no project is left alone",
			existing: seededActivity{ID: "a1"},
			act:      demoActivity{Company: "nobody.test", DaysAgo: 30},
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
