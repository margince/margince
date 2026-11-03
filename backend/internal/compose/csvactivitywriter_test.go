// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestARowWithoutATypeOrADateIsNotFiled(t *testing.T) {
	w := &csvWriters{object: migration.ObjectActivity, sourceSystem: importSourceSystem("crm-old")}
	when := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)
	row := migration.Row{ExternalID: "A-1", Fields: map[string]any{
		activityTargetOccurredAt: "2025-03-02", activityTargetSubject: "Intro call notes",
	}}
	cases := map[string]struct {
		typed map[string]any
		want  migration.ReasonCode
	}{
		"no type":    {map[string]any{activityTargetOccurredAt: when}, migration.ReasonActivityTypeUnnamed},
		"no date":    {map[string]any{activityTargetType: "note"}, migration.ReasonOccurredAtUnnamed},
		"both named": {map[string]any{activityTargetType: "note", activityTargetOccurredAt: when}, ""},
	}
	empty := migration.Row{ExternalID: "A-2", Fields: map[string]any{activityTargetBody: "  ", activityTargetOccurredAt: "2025-03-02"}}
	for _, kind := range []string{"note", "call"} {
		typed := map[string]any{activityTargetType: kind, activityTargetOccurredAt: when}
		if _, why := w.activityBirthOf(empty, typed); why.Code != migration.ReasonActivityEmpty {
			t.Errorf("an empty %s: reason = %q, want %q, so the dry run and the commit skip it alike", kind, why.Code, migration.ReasonActivityEmpty)
		}
	}
	meeting := map[string]any{activityTargetType: "meeting", activityTargetOccurredAt: when}
	if _, why := w.activityBirthOf(empty, meeting); why.Code != "" {
		t.Errorf("a meeting with no subject or body: reason = %q, want it filed", why.Code)
	}
	for name, c := range cases {
		in, why := w.activityBirthOf(row, c.typed)
		if why.Code != c.want {
			t.Errorf("%s: reason = %q, want %q", name, why.Code, c.want)
		}
		if c.want == "" && (in.Kind != "note" || in.Source != w.provenanceOf("A-1")) {
			t.Errorf("%s: input = %+v, want a note carrying the row's provenance", name, in)
		}
	}
}

func TestEachRefusalOfTheActivitiesStoreIsTheReasonTheReportShows(t *testing.T) {
	note := "held"
	kindFault := activities.CheckKindFields("note", nil, &note, "")
	if kindFault == nil {
		t.Fatal("a meeting status on a note must be refused by the store's own check")
	}
	cases := map[string]struct {
		err  error
		want migration.ReasonCode
	}{
		"empty entry":     {fmt.Errorf("filing: %w", &activities.EmptyEntryError{Kind: "note"}), migration.ReasonActivityEmpty},
		"too many links":  {&activities.TooManyLinksError{Count: activities.MaxLinks + 1}, migration.ReasonTooManyLinks},
		"company meeting": {&activities.CompanyMeetingError{Kind: "meeting"}, migration.ReasonCompanyLinkOnMeeting},
		"kind rule":       {fmt.Errorf("filing: %w", kindFault), migration.ReasonMeetingStatusNotForKind},
		"not taken place": {fmt.Errorf("filing: %w", &activities.MeetingNotTakenPlaceError{}), migration.ReasonMeetingNotTakenPlace},
		"retention hold":  {fmt.Errorf("filing: %w", apperrors.ErrRetentionHold), migration.ReasonActivityHeld},
		"not permitted":   {apperrors.ErrPermissionDenied, migration.ReasonLinkNotAttachable},
		"not visible":     {apperrors.ErrNotFound, migration.ReasonLinkNotAttachable},
	}
	for name, c := range cases {
		why, refused := activityRefusalOf(c.err, map[string]string{})
		if !refused || why.Code != c.want {
			t.Errorf("%s: got (%q, %v), want %q refused", name, why.Code, refused, c.want)
		}
	}
	for name, err := range map[string]error{"no error": nil, "an unknown fault": errors.New("disk full")} {
		if why, refused := activityRefusalOf(err, nil); refused {
			t.Errorf("%s: refused as %q, want it left to stop the run", name, why.Code)
		}
	}
}
