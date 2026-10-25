// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// activityRoundTrip is the activity arm of TestEveryImportTargetRoundTripsThroughCreateAndUpdate:
// which targets reach the create input and which the update.
func activityRoundTrip(fields map[string]string, typed map[string]any) (created, patched map[string]bool) {
	typed[activityTargetType] = "meeting"
	typed[activityTargetOccurredAt] = time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	typed[activityTargetDuration] = 1800
	typed[activityTargetMeetingStatus] = "held"
	typed[activityTargetDirection] = "outbound"
	typed[activityTargetLinks] = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.NewV7()}}
	in := activityCreateFrom(fields, typed, importSourceSystem(""), "src")
	up := activityUpdateFrom(fields, typed)
	created = map[string]bool{
		"type":             in.Kind != "",
		"subject":          in.Subject != nil,
		"body":             in.Body != nil,
		"occurred_at":      in.OccurredAt != nil,
		"duration_minutes": in.DurationSeconds != nil,
		"meeting_status":   in.MeetingStatus != nil,
		"direction":        in.Direction != nil,
		"links":            len(in.Links) > 0,
		"author":           in.Author.AuthorName != nil,
		"owner_email":      in.Author.AuthorID != nil,
	}
	patched = map[string]bool{
		"subject":          up.Subject != nil,
		"body":             up.Body != nil,
		"occurred_at":      up.OccurredAt != nil,
		"duration_minutes": up.DurationSeconds != nil,
		"meeting_status":   up.MeetingStatus != nil,
	}
	return created, patched
}

func TestAnActivityCreateLeavesTheSourceKeyEmpty(t *testing.T) {
	in := activityCreateFrom(map[string]string{"ref": "A-1"}, map[string]any{activityTargetType: "note"},
		importSourceSystem("crm-old"), "mirror:csv:crm-old:activity:A-1")
	if in.SourceID != nil {
		t.Fatalf("SourceID = %q, want none: idempotency lives in the identity map, and a source key would replay an archived row", *in.SourceID)
	}
	if in.SourceSystem == nil || *in.SourceSystem != importSourceSystem("crm-old") {
		t.Fatalf("SourceSystem = %v, want the run's namespace", in.SourceSystem)
	}
}

func TestAnUneditedTimestampIsNotAChangeWhateverZoneTheStoreAnswersIn(t *testing.T) {
	stored := []byte(`{"occurred_at":"2025-03-02T07:00:00+07:00"}`)
	typed := map[string]any{activityTargetOccurredAt: time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)}
	changed, err := changedFields(stored, map[string]string{activityTargetOccurredAt: "2025-03-02"}, typed)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v, %v; want none: the stored instant is the cell's instant", changed, err)
	}
}

// A day cell moves a stored activity to a day, because the store reads a new
// time sent alone as an instant. A timed cell says nothing, which is that instant.
func TestADayCellCarriesItsPrecisionToTheCreateAndTheUpdate(t *testing.T) {
	for cell, want := range map[string]string{"2025-03-09": activities.PrecisionDay, "2025-03-09T10:00:00Z": ""} {
		fields := map[string]string{activityTargetOccurredAt: cell}
		typed, problem := checkActivityCells(fields, nil, cellClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), zone: time.UTC})
		if problem != nil {
			t.Fatalf("%s: %+v", cell, problem)
		}
		if got := activityCreateFrom(fields, typed, importSourceSystem(""), "src").OccurredPrecision; got != want {
			t.Errorf("%s creates as %q, want %q", cell, got, want)
		}
		got := ""
		if p := activityUpdateFrom(fields, typed).OccurredPrecision; p != nil {
			got = *p
		}
		if got != want {
			t.Errorf("%s updates as %q, want %q", cell, got, want)
		}
	}
}
