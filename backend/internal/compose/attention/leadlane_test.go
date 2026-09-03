// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The lead lane: an inbound lead nobody has replied to reaches the same ranked
// day as everything else, and says which clock it is against.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type stubLeads struct {
	rows    []OwedLead
	tracked bool
	err     error
}

func (s *stubLeads) Owed(context.Context, TaskScope, ids.UUID, int) ([]OwedLead, bool, error) {
	return s.rows, s.tracked, s.err
}

func leadReader() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-000000000001"),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

func leadLaneService(leads LeadResponses) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock).WithLeadResponses(leads)
}

func rowFor(t *testing.T, day crmcontracts.Worklist, title string) crmcontracts.WorklistItem {
	t.Helper()
	for _, row := range day.Queue {
		if row.Title != nil && *row.Title == title {
			return row
		}
	}
	t.Fatalf("no row titled %q on the queue", title)
	return crmcontracts.WorklistItem{}
}

func kindsOf(row crmcontracts.WorklistItem) []string {
	out := make([]string, 0, len(row.Because))
	for _, because := range row.Because {
		out = append(out, string(because.Kind))
	}
	return out
}

// The order is the whole claim: a lead whose reply deadline has passed is
// somebody outside waiting, and it belongs beside a waiting customer rather
// than below the routine work.
func TestABreachedLeadRanksAboveOneMerelyAtRisk(t *testing.T) {
	svc := leadLaneService(&stubLeads{tracked: true, rows: []OwedLead{
		{
			ID: ids.NewV7(), Name: "at risk", DeadlineAt: readInstant.Add(30 * time.Minute),
			State: "at_risk", OwnerID: ids.NewV7(),
		},
		{
			ID: ids.NewV7(), Name: "breached", DeadlineAt: readInstant.Add(-2 * time.Hour),
			State: "breached", OwnerID: ids.NewV7(),
		},
	}})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	breached := rowFor(t, day, "breached")
	atRisk := rowFor(t, day, "at risk")
	if breached.Level >= atRisk.Level {
		t.Errorf("breached ranked at level %d and at-risk at %d — the overdue one must lead",
			breached.Level, atRisk.Level)
	}
	if got := kindsOf(breached); !slices.Contains(got, "response_overdue") {
		t.Errorf("a breached lead said %v, wanted response_overdue among them", got)
	}
	if got := kindsOf(atRisk); !slices.Contains(got, "response_due_soon") {
		t.Errorf("an at-risk lead said %v, wanted response_due_soon among them", got)
	}
	if breached.Category != "leads" {
		t.Errorf("a lead landed in category %q, wanted leads", breached.Category)
	}
}

// An at-risk lead says WHEN, not merely that something is due.
//
// "reply due soon" alone asks the rep to guess how soon, while its breached
// sibling already carries a figure. The moment travels as a date value and the
// client formats it — nothing here composes a date into words, because the
// product ships three languages and a zone per reader.
func TestAnAtRiskLeadCarriesTheDeadlineItIsMeasuredAgainst(t *testing.T) {
	due := readInstant.Add(30 * time.Minute)
	svc := leadLaneService(&stubLeads{tracked: true, rows: []OwedLead{
		{ID: ids.NewV7(), Name: "at risk", DeadlineAt: due, State: "at_risk", OwnerID: ids.NewV7()},
	}})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	row := rowFor(t, day, "at risk")
	for _, because := range row.Because {
		if because.Kind != "response_due_soon" {
			continue
		}
		if because.Value == nil {
			t.Fatal("the at-risk reason carries no value, so the row says a reply is due " +
				"soon without saying by when")
		}
		if because.Value.Kind != "date" || because.Value.Date == nil {
			t.Fatalf("the value is %+v, wanted a date — the client renders a date in the "+
				"reader's own locale and zone, and no other kind reaches that path",
				because.Value)
		}
		if !because.Value.Date.Equal(due) {
			t.Errorf("the deadline reads %s, wanted the lead's own %s",
				because.Value.Date, due)
		}
		return
	}
	t.Fatalf("no response_due_soon reason on the row: %v", kindsOf(row))
}

// A lead with no deadline says the plain sentence rather than a zero time.
//
// The honest absence: a date value built from an unset time would render as
// 1 January, year 1, which is a worse answer than not naming a moment at all.
func TestAnAtRiskLeadWithNoDeadlineNamesNoMoment(t *testing.T) {
	svc := leadLaneService(&stubLeads{tracked: true, rows: []OwedLead{
		{ID: ids.NewV7(), Name: "at risk", State: "at_risk", OwnerID: ids.NewV7()},
	}})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	row := rowFor(t, day, "at risk")
	for _, because := range row.Because {
		if because.Kind == "response_due_soon" && because.Value != nil {
			t.Errorf("a lead with no deadline still carried a value: %+v", because.Value)
		}
	}
}

// The policy question is not a filter. With no target set nothing is late, so
// the source is ABSENT — a page reporting zero overdue leads would be stating a
// number nothing measures.
func TestWithNoFirstResponseTargetTheLaneClaimsNothing(t *testing.T) {
	svc := leadLaneService(&stubLeads{tracked: false, rows: []OwedLead{
		{ID: ids.NewV7(), Name: "a lead", State: "breached", DeadlineAt: readInstant.Add(-time.Hour)},
	}})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	for _, row := range day.Queue {
		if row.Source == sourceLeadResponse {
			t.Fatalf("a lead row reached the queue with the first-response target switched off")
		}
	}
	for _, missing := range day.SourcesUnavailable {
		if missing.Source == sourceLeadResponse {
			t.Fatalf("the lane reported itself unavailable; an unmeasured target is not a failed read")
		}
	}
	for _, count := range day.Counts {
		if count.Category == "leads" && count.Considered > 0 {
			t.Fatalf("the leads category counted %d rows nothing measured", count.Considered)
		}
	}
}

// A lead with no owner is somebody's to pick up, and the row says so on top of
// whatever its clock says.
func TestAnUnassignedLeadNamesThatItAnswersToNobody(t *testing.T) {
	svc := leadLaneService(&stubLeads{tracked: true, rows: []OwedLead{
		{ID: ids.NewV7(), Name: "nobody's", DeadlineAt: readInstant.Add(-time.Hour), State: "breached"},
	}})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if got := kindsOf(rowFor(t, day, "nobody's")); !slices.Contains(got, "unassigned") {
		t.Errorf("an ownerless lead said %v, wanted unassigned among them", got)
	}
}

// A named owner's page keeps the lane. The lead lane narrowed to that person in
// its own query, so the row is already theirs — judging it by the deal arm
// would drop every one and the manager would find the lane silently missing
// rather than empty.
func TestANamedOwnersQueueKeepsTheirOwedLeads(t *testing.T) {
	rep := ids.MustParse("01a05500-0000-7000-8000-00000000c0fe")
	svc := leadLaneService(&stubLeads{tracked: true, rows: []OwedLead{
		{
			ID: ids.NewV7(), Name: "their lead", DeadlineAt: readInstant.Add(-time.Hour),
			State: "breached", OwnerID: rep,
		},
	}})

	day, err := svc.Worklist(leadReader(), "", "", rep, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	rowFor(t, day, "their lead")
}

// With the target switched off the source must be ABSENT, and a reach row is
// not absence: reachOf emits one for every bounded-source key, so recording the
// lane unconditionally published a zero-valued entry that reads as "read, and
// there was nothing".
func TestAnUntrackedLanePublishesNoReachRow(t *testing.T) {
	svc := leadLaneService(&stubLeads{tracked: false})

	day, err := svc.Worklist(leadReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	for _, reach := range day.Reach {
		if reach.Source == sourceLeadResponse {
			t.Fatalf("the lane published a reach row (considered=%d shown=%d) with nothing measuring first response",
				reach.Considered, reach.Shown)
		}
	}
}
