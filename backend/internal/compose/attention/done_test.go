// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func withActions(verbs ...crmcontracts.AttentionItemActions) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.Actions = verbs }
}

func withMoved(at time.Time) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.SubjectMovedAt = &at }
}

func withQuietDays(days int) func(*crmcontracts.AttentionItem) {
	return func(i *crmcontracts.AttentionItem) { i.QuietDays = &days }
}

// queueOf assembles one reader's page with the given rows marked done.
func queueOf(t *testing.T, day crmcontracts.Attention, done map[RowRef]time.Time) []crmcontracts.WorklistItem {
	t.Helper()
	return narrowedQueueOf(t, day, done, "")
}

// narrowedQueueOf is queueOf under a filter. A filter that opens the folded
// groups skips the fold, so it is the path where only the first pass applies.
func narrowedQueueOf(
	t *testing.T, day crmcontracts.Attention, done map[RowRef]time.Time, filter string,
) []crmcontracts.WorklistItem {
	t.Helper()
	svc := &Service{marked: doneState{at: done}}
	return svc.worklistFrom(t.Context(), day, scopeAll, filter, 50, waitingRead{}, worklistCursor{}, nil).Queue
}

func hasRow(queue []crmcontracts.WorklistItem, source crmcontracts.WorklistItemSource, id string) bool {
	return slices.ContainsFunc(queue, func(row crmcontracts.WorklistItem) bool {
		return row.Source == source && row.Id == id
	})
}

// A deal at risk the reader marked done stays off their list while nothing
// happens on it. It comes back on activity after the press, or once its close
// date has passed by the overdue rule.
func TestADealMarkedDoneComesBackWhenItMoves(t *testing.T) {
	t.Parallel()
	pressed := rankInstant.Add(-3 * 24 * time.Hour)
	done := map[RowRef]time.Time{{Source: sourceAtRisk, RowID: "deal"}: pressed}
	day := func(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }
	cases := []struct {
		name  string
		risk  crmcontracts.AttentionItem
		shown bool
	}{
		{"quiet since before the press", item("deal", "deal_at_risk", withQuietDays(40),
			withMoved(pressed.Add(-40*24*time.Hour)), withDue(day(30))), false},
		{"activity after the press, quiet again", item("deal", "deal_at_risk", withQuietDays(1),
			withMoved(pressed.Add(time.Hour))), true},
		{"close date passed after the press", item("deal", "deal_at_risk", withQuietDays(40),
			withMoved(pressed.Add(-40*24*time.Hour)), withDue(day(23)), withOverdue(true)), true},
		{"close date is the press day, so it passed after it", item("deal", "deal_at_risk",
			withQuietDays(40), withDue(day(22)), withOverdue(true)), true},
		{"close date passed before the press", item("deal", "deal_at_risk", withQuietDays(40),
			withDue(day(21)), withOverdue(true)), false},
	}
	for _, tc := range cases {
		day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: lane(tc.risk)}
		if !hasRow(queueOf(t, day, nil), sourceAtRisk, "deal") {
			t.Fatalf("%s: the deal is not on the page unmarked, so this case proves nothing", tc.name)
		}
		if got := hasRow(queueOf(t, day, done), sourceAtRisk, "deal"); got != tc.shown {
			t.Errorf("%s: the deal is shown = %v, want %v", tc.name, got, tc.shown)
		}
	}
}

// A capture problem with no streak start comes back once the connection
// synced again after the press: the concern cleared and returned.
func TestACaptureConcernComesBackAfterTheConnectionRecovered(t *testing.T) {
	t.Parallel()
	pressed := rankInstant.Add(-2 * time.Hour)
	done := map[RowRef]time.Time{{Source: "capture_health", RowID: "conn"}: pressed}
	for _, tc := range []struct {
		synced time.Time
		shown  bool
	}{{pressed.Add(-time.Hour), false}, {pressed.Add(time.Hour), true}} {
		concern := item("conn", "capture_health", withKind("reauth_required"), withMoved(tc.synced))
		day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: lane(concern)}
		if got := hasRow(queueOf(t, day, done), "capture_health", "conn"); got != tc.shown {
			t.Errorf("last synced %v: shown = %v, want %v", tc.synced, got, tc.shown)
		}
	}
}

// A health row comes back on a failure newer than the press, and a meeting
// marked done stays off the list once it has started.
func TestARowComesBackOnlyOnSomethingNewOnItsSubject(t *testing.T) {
	t.Parallel()
	pressed := rankInstant.Add(-2 * time.Hour)
	done := map[RowRef]time.Time{
		{Source: "bounce", RowID: "old"}:      pressed,
		{Source: "bounce", RowID: "new"}:      pressed,
		{Source: sourceMeeting, RowID: "met"}: pressed,
	}
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Bounces: lane(
			item("old", "bounce", withOccurred(pressed.Add(-time.Hour))),
			item("new", "bounce", withOccurred(pressed.Add(time.Hour))),
		),
		Meetings: lane(item("met", "meeting", withOccurred(pressed.Add(time.Hour)),
			withDue(rankInstant.Add(time.Hour)))),
	}
	plain := queueOf(t, day, nil)
	for _, id := range []string{"old", "new"} {
		if !hasRow(plain, "bounce", id) {
			t.Fatalf("bounce %q is not on the page unmarked, so this test proves nothing", id)
		}
	}
	if !hasRow(plain, sourceMeeting, "met") {
		t.Fatal("the meeting is not on the page unmarked, so this test proves nothing")
	}
	queue := queueOf(t, day, done)
	if hasRow(queue, "bounce", "old") {
		t.Error("a bounce marked done after it happened came back with nothing new")
	}
	if !hasRow(queue, "bounce", "new") {
		t.Error("a bounce that failed again after the press stayed hidden")
	}
	if hasRow(queue, sourceMeeting, "met") {
		t.Error("a meeting marked done came back when it started")
	}
	if hasRow(narrowedQueueOf(t, day, done, string(categorySystem)), "bounce", "old") {
		t.Error("a bounce marked done came back under the system filter, which skips the fold")
	}
}

// A folded group can be marked done, though the fold mints its id after the
// lanes are read. It comes back when a newer member joins it.
func TestAGroupMarkedDoneComesBackOnANewMember(t *testing.T) {
	t.Parallel()
	pressed := rankInstant.Add(-time.Hour)
	groupOf := func(newest time.Time) crmcontracts.Attention {
		needs := make([]crmcontracts.AttentionItem, 0, 12)
		for i := range 12 {
			needs = append(needs, item(string(rune('a'+i)), "approval",
				withKind("capture_counterparty"), withOccurred(pressed.Add(-24*time.Hour))))
		}
		needs[0].OccurredAt = &newest
		return crmcontracts.Attention{AsOf: rankInstant, NeedsYou: needs}
	}
	quiet := groupOf(pressed.Add(-2 * time.Hour))
	var groupID string
	for _, row := range queueOf(t, quiet, nil) {
		if row.Source == "batch" {
			groupID = row.Id
		}
	}
	if groupID == "" {
		t.Fatal("nothing folded, so this fixture cannot test a group's Done")
	}
	done := map[RowRef]time.Time{{Source: "batch", RowID: groupID}: pressed}
	if hasRow(queueOf(t, quiet, done), "batch", groupID) {
		t.Error("a group marked done is still on the page: its id is minted by the fold, " +
			"so a Done applied only before the fold matches nothing")
	}
	if !hasRow(queueOf(t, groupOf(pressed.Add(30*time.Minute)), done), "batch", groupID) {
		t.Error("a group whose newest member arrived after the press stayed hidden")
	}
}

// Done is offered on the rows that had no way off the list, and nowhere a
// row's own verb already finishes it.
func TestDoneIsOfferedWhereTheRowHasNoOtherWayOff(t *testing.T) {
	t.Parallel()
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		AtRisk:   lane(item("deal", "deal_at_risk", withQuietDays(40))),
		Bounces:  lane(item("bounced", "bounce", withOccurred(rankInstant.Add(-time.Hour)))),
		Planned:  []crmcontracts.AttentionItem{item("task", "task", withActions("complete", "snooze"))},
		NeedsYou: []crmcontracts.AttentionItem{item("approval", "approval", decidable())},
	}
	want := map[string]bool{"deal": true, "bounced": true, "task": false, "approval": false}
	for _, row := range queueOf(t, day, nil) {
		offers, asked := want[row.Id]
		if !asked {
			continue
		}
		delete(want, row.Id)
		got := row.Dispositions != nil && slices.Equal(*row.Dispositions,
			[]crmcontracts.WorklistItemDispositions{disposeHandled})
		if got != offers {
			t.Errorf("%s row %q offers Done alone = %v, want %v", row.Source, row.Id, got, offers)
		}
	}
	if len(want) != 0 {
		t.Fatalf("the page drew none of %v, so this test judged nothing about them", want)
	}
}

func TestDomainReviewEvidenceDoesNotCreatePersonalDuty(t *testing.T) {
	pressed := rankInstant.Add(-time.Hour)
	old := pressed.Add(-time.Hour)
	fresh := pressed.Add(time.Minute)
	for _, tc := range []struct {
		name     string
		evidence *time.Time
		shown    bool
	}{
		{"undated evidence", nil, false}, {"same evidence rescanned", &old, false}, {"new evidence", &fresh, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			question := domainQuestionItem(DomainQuestion{Domain: "example.test", AskedAt: rankInstant, EvidenceAt: tc.evidence})
			day := crmcontracts.Attention{AsOf: rankInstant, DomainQuestions: lane(question)}
			done := map[RowRef]time.Time{{Source: "domain_question", RowID: "example.test"}: pressed}
			if got := hasRow(queueOf(t, day, done), "domain_question", "example.test"); got != tc.shown {
				t.Fatalf("question shown = %v, want %v", got, tc.shown)
			}
		})
	}
}
