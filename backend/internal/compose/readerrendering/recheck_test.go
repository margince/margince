// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// movingActivity answers the activity as it was on the first read. Every read
// after answers what `later` makes of it: the row changing while the model writes.
type movingActivity struct {
	activity crmcontracts.Activity
	later    func(crmcontracts.Activity) (crmcontracts.Activity, error)
	reads    int
}

func (m *movingActivity) GetActivityContent(context.Context, ids.ActivityID, storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	m.reads++
	if m.reads == 1 {
		return m.activity, nil
	}
	return m.later(m.activity)
}

func renderMoving(t *testing.T, rows *movingActivity, lane *recordingLane) (crmcontracts.ActivityReaderRendering, error) {
	t.Helper()
	now := clocktest.Now(t)
	svc := NewService(nil, rows, func() time.Time { return now }).WithLane(lane)
	return svc.Render(reader("en"), ids.From[ids.ActivityKind](ids.UUID(rows.activity.Id)), nil)
}

func TestARowThatChangedWhileTheModelWroteShowsNoRendering(t *testing.T) {
	version := crmcontracts.RowVersion(3)
	cases := map[string]func(crmcontracts.Activity) (crmcontracts.Activity, error){
		"archived, held or out of the audience": func(crmcontracts.Activity) (crmcontracts.Activity, error) {
			return crmcontracts.Activity{}, apperrors.ErrNotFound
		},
		"withheld from the reader": func(a crmcontracts.Activity) (crmcontracts.Activity, error) {
			a.ContentState = new(crmcontracts.ActivityContentStateWithheld)
			return a, nil
		},
		"edited": func(a crmcontracts.Activity) (crmcontracts.Activity, error) {
			a.Body = new(germanBody + " Nachtrag.")
			return a, nil
		},
		"a new version": func(a crmcontracts.Activity) (crmcontracts.Activity, error) {
			a.Version = new(version + 1)
			return a, nil
		},
	}
	for name, later := range cases {
		activity := note(crmcontracts.ActivityKindNote, germanBody, nil)
		activity.Version = &version
		lane := &recordingLane{reply: englishReply}
		got, err := renderMoving(t, &movingActivity{activity: activity, later: later}, lane)
		if !errors.Is(err, apperrors.ErrNotFound) || got.Body != nil {
			t.Errorf("%s: rendering %+v, %v, want ErrNotFound and nothing shown", name, got, err)
		}
		if len(lane.sent) != 1 {
			t.Errorf("%s: the model was asked %d times, want once before the row moved", name, len(lane.sent))
		}
	}

	unmoved := note(crmcontracts.ActivityKindNote, germanBody, nil)
	unmoved.Version = &version
	same := func(a crmcontracts.Activity) (crmcontracts.Activity, error) { return a, nil }
	if got, err := renderMoving(t, &movingActivity{activity: unmoved, later: same}, &recordingLane{reply: englishReply}); err != nil || got.Body == nil {
		t.Errorf("a row that did not move is refused: %+v, %v", got, err)
	}
}

func TestTextPastTheLimitIsRefusedEvenInTheReadersLanguage(t *testing.T) {
	de := crmcontracts.ActivityLanguageDe
	long := strings.Repeat(germanBody+" ", MaxSourceRunes/len([]rune(germanBody))+1)
	lane := &recordingLane{reply: englishReply}
	if _, err := render(reader("de"), t, note(crmcontracts.ActivityKindNote, long, &de), lane); !errors.Is(err, ErrTooLong) {
		t.Errorf("an oversized German note for a German reader: %v, want ErrTooLong", err)
	}
	if len(lane.sent) != 0 {
		t.Error("an oversized note reached the model")
	}
}

func TestTheTextsOwnLanguageWinsOverAStaleLabel(t *testing.T) {
	stale := crmcontracts.ActivityLanguageDe
	english := "We talked about the renewal. They want to see the offer by Friday, and purchasing is no longer against the price."
	lane := &recordingLane{reply: englishReply}
	got, err := render(reader("en"), t, note(crmcontracts.ActivityKindNote, english, &stale), lane)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Unchanged || got.SourceLanguage == nil || *got.SourceLanguage != "en" || len(lane.sent) != 0 {
		t.Errorf("an English note labelled de, read in English: %+v after %d calls, want unchanged with no call", got, len(lane.sent))
	}

	unplaced := note(crmcontracts.ActivityKindNote, "Termin ok.", &stale)
	unplaced.Subject = new("Ok")
	if got, err := render(reader("de"), t, unplaced, &recordingLane{}); err != nil || !got.Unchanged {
		t.Errorf("text the detector cannot place keeps its stored label: %+v, %v", got, err)
	}
}
