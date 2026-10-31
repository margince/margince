// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// windowRecorder is a task backlog that answers nothing and keeps the window
// each continuation asked for.
type windowRecorder struct{ asked []TaskWindow }

func (w *windowRecorder) CountTasks(context.Context, TaskWindow) (int, error) { return 0, nil }

func (w *windowRecorder) TasksAfter(_ context.Context, window TaskWindow, _ string, _ int) ([]Positioned[Task], bool, error) {
	w.asked = append(w.asked, window)
	return nil, false, nil
}

func (w *windowRecorder) DealsWithTaskDue(context.Context, TaskWindow, []ids.UUID) (map[ids.UUID]bool, error) {
	return nil, nil
}

func tailToken(t *testing.T, rest *backlogCursor) string {
	t.Helper()
	token, err := storekit.EncodeOpaque(worklistCursor{At: 3, Params: fingerprint(scopeMine, "", ids.Nil), Rest: rest})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestATailThisBinaryCouldNotHaveMintedIsRefused(t *testing.T) {
	window := tailWindow{AsOf: rankInstant, Until: rankInstant.Add(time.Hour), Zone: "UTC"}
	position, err := storekit.EncodeOpaque(map[string]string{"id": ids.NewV7().String()})
	if err != nil {
		t.Fatal(err)
	}
	refused := map[string]*backlogCursor{
		"an unknown source":      {At: map[string]string{"meeting": ""}, Window: window},
		"a position that is not": {At: map[string]string{"task": "not a token"}, Window: window},
		"no source at all":       {At: map[string]string{}, Window: window},
		"no window":              {At: map[string]string{"task": ""}},
		"a zone nobody can load": {At: map[string]string{"task": ""}, Window: tailWindow{AsOf: window.AsOf, Until: window.Until, Zone: "Mars/Olympus"}},
	}
	for name, rest := range refused {
		_, err := decodeCursor(tailToken(t, rest), scopeMine, "", ids.Nil)
		if !isMismatch(err) {
			t.Errorf("%s: decoded with %v, want the mismatch refusal", name, err)
		}
	}
	for name, rest := range map[string]*backlogCursor{
		"a tail at its beginning": {At: map[string]string{"task": ""}, Window: window},
		"a tail with a position":  {At: map[string]string{"task": position}, Window: window},
		"no tail":                 nil,
	} {
		if _, err := decodeCursor(tailToken(t, rest), scopeMine, "", ids.Nil); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}

// A binary from before the tail fingerprinted the question alone. Its tokens
// are refused here, and it would refuse ours, so neither silently drops a tail.
func TestTheTailProtocolIsInTheFingerprint(t *testing.T) {
	sum := sha256.Sum256([]byte(scopeMine + "\x00" + "" + "\x00" + ids.Nil.String()))
	older := hex.EncodeToString(sum[:8])
	if older == fingerprint(scopeMine, "", ids.Nil) {
		t.Fatal("the fingerprint does not carry the tail protocol, so an older binary accepts a tailed token")
	}
	token, err := storekit.EncodeOpaque(worklistCursor{At: 3, Params: older})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeCursor(token, scopeMine, "", ids.Nil); !isMismatch(err) {
		t.Errorf("an older binary's token decoded with %v, want the mismatch refusal", err)
	}
}

// A walk that crosses midnight keeps reading the window it began in: the
// keyset it carries is a position in that window and no other.
func TestTheTailReadsTheWindowItsWalkBeganIn(t *testing.T) {
	recorder := &windowRecorder{}
	svc := (&Service{}).WithTaskBacklog(recorder)
	before := time.Date(2026, 10, 30, 23, 55, 0, 0, time.UTC)
	after := before.Add(10 * time.Minute)
	bounded := map[crmcontracts.WorklistItemSource]bool{sourceTask: true}

	first := svc.readingBacklog(t.Context(), crmcontracts.Attention{AsOf: before}, before.Truncate(24*time.Hour).Add(24*time.Hour), time.UTC, true)
	rest := first.startTail(tailPage{scope: scopeMine, day: crmcontracts.Attention{AsOf: before}, bounded: bounded, first: true})
	if rest == nil {
		t.Fatal("a cut task lane started no tail")
	}
	later := svc.readingBacklog(t.Context(), crmcontracts.Attention{AsOf: after}, after.Truncate(24*time.Hour).Add(24*time.Hour), time.UTC, false)
	later.backlogTail(t.Context(), tailPage{scope: scopeMine, day: crmcontracts.Attention{AsOf: after}}, 5, rest, nil)

	if len(recorder.asked) != 1 {
		t.Fatalf("the tail read %d windows, want 1", len(recorder.asked))
	}
	if got := recorder.asked[0]; !got.AsOf.Equal(before) || !got.Until.Equal(before.Truncate(24*time.Hour).Add(24*time.Hour)) {
		t.Errorf("past midnight the tail read the window as of %s until %s, want the walk's own day", got.AsOf, got.Until)
	}
}

func isMismatch(err error) bool {
	_, mismatch := errors.AsType[*storekit.CursorSortMismatchError](err)
	return mismatch
}

// laterCount is a task backlog whose count of the later window answers err.
type laterCount struct {
	windowRecorder
	err error
}

func (c *laterCount) CountTasks(context.Context, TaskWindow) (int, error) { return 9, c.err }

// A count that fails publishes no total for its category, which stays a floor
// the page still says has more. It never publishes the failed count's number.
func TestAFailedCountLeavesItsCategoryAFloorAndNoPartialTotal(t *testing.T) {
	due := readInstant.AddDate(0, 0, 3)
	var later []Task
	for range upcomingCap {
		later = append(later, Task{ID: ids.NewV7(), Subject: "Later", DueAt: &due})
	}
	for _, tc := range []struct {
		name string
		err  error
		want *int
	}{
		{"a count that answers", nil, new(9)},
		{"a count that fails", context.DeadlineExceeded, nil},
	} {
		svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{upcoming: later}, stubReceipts{}, stubBriefing{},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithTaskBacklog(&laterCount{err: tc.err})
		day, err := svc.Worklist(pageReader(), "", "", ids.UUID{}, 25, "")
		if err != nil {
			t.Fatalf("%s: the page failed: %v", tc.name, err)
		}
		var upcoming *crmcontracts.WorklistCount
		for i := range day.Counts {
			if day.Counts[i].Category == crmcontracts.WorklistCountCategory(categoryUpcoming) {
				upcoming = &day.Counts[i]
			}
		}
		total := "none"
		if upcoming != nil && upcoming.Total != nil {
			total = fmt.Sprint(*upcoming.Total)
		}
		switch {
		case upcoming == nil:
			t.Errorf("%s: the page publishes no upcoming count", tc.name)
		case tc.want == nil && (upcoming.Total != nil || !upcoming.MoreAvailable):
			t.Errorf("%s: upcoming publishes total %s, more %v; want no total and more available",
				tc.name, total, upcoming.MoreAvailable)
		case tc.want != nil && (upcoming.Total == nil || *upcoming.Total != *tc.want):
			t.Errorf("%s: upcoming publishes total %s, want %d", tc.name, total, *tc.want)
		}
	}
}
