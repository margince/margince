// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A plan that reads an activity twice asks the content gate of both rows. A
// note whose source meeting is limited to its participants stays out of reach
// through that meeting. Neither a hop nor a filter on its reference finds it.
// A note, not a task: a task's own gate already inherits its source's audience.
// Rep3 holds the meeting, so its absence for Rep1 is the audience's doing.
func TestAPlanReadsAnActivityThroughAnotherOnlyWhenItMayReadBoth(t *testing.T) {
	q := setupQuery(t)
	q.seedFixture(t)
	meeting := func(audience string, source *ids.UUID) ids.UUID {
		return q.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience, source_activity_id)
			VALUES ($1, 'meeting', 'Board review', now(), 'manual', 'human:'||$2::text, $3, $4)`, q.Rep3.String(), audience, source)
	}
	limited, open := meeting("participants", nil), meeting("workspace", nil)
	// An open follow-up meeting held over from the limited one. From a note
	// about the follow-up, its reference is the only way to the limited one.
	followUp := meeting("workspace", &limited)
	note := func(source ids.UUID) ids.UUID {
		return q.SeedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience, source_activity_id)
			VALUES ($1, 'note', 'Minutes', now(), 'manual', 'human:'||$2::text, 'workspace', $3)`, q.Rep1.String(), source)
	}
	fromLimited, fromOpen, fromFollowUp := note(limited), note(open), note(followUp)

	answered := func(ctx context.Context, doc string) map[ids.UUID]bool {
		t.Helper()
		got := map[ids.UUID]bool{}
		for _, row := range q.run(ctx, t, doc).Rows {
			got[row.ID] = true
		}
		return got
	}
	rep1, rep3 := q.teamRep(q.Rep1, q.Team1), q.teamRep(q.Rep3, q.Team2)
	hop := `{"version": "v1", "target": "activity", "traverse": {"relation": "source_activity"}}`
	bySource := func(source ids.UUID) string {
		return fmt.Sprintf(`{"version": "v1", "target": "activity", "where": [{"field": "source_activity_id", "op": "eq", "value": %q}]}`, source)
	}
	inverse := `{"version": "v1", "target": "activity", "traverse": {"relation": "activities_by_source_activity"}}`
	hopBySource := fmt.Sprintf(`{"version": "v1", "target": "activity", "traverse": {"relation": "source_activity",
		"where": [{"field": "source_activity_id", "op": "eq", "value": %q}]}}`, limited)

	if got := answered(rep3, hop); !got[fromLimited] || !got[fromOpen] {
		t.Fatalf("the meeting's holder does not reach both notes through their meetings, so the fixture proves nothing: %v", got)
	}
	if got := answered(rep3, hopBySource); !got[fromFollowUp] {
		t.Fatalf("the meeting's holder does not reach the follow-up's note by the limited meeting's id: %v", got)
	}
	if got := answered(rep1, hopBySource); got[fromFollowUp] {
		t.Errorf("a hop filtered on a limited meeting's id answered %v; the filter discloses the meeting", got)
	}
	if got := answered(rep1, hop); got[fromLimited] || !got[fromOpen] {
		t.Errorf("a hop onto the source meeting answered %v; want the open meeting's note and not the limited one's", got)
	}
	if got := answered(rep1, bySource(limited)); len(got) != 0 {
		t.Errorf("filtering on a limited meeting's id answered %v; the filter discloses the meeting", got)
	}
	if got := answered(rep1, bySource(open)); !got[fromOpen] {
		t.Errorf("filtering on an open meeting's id withheld its note: %v", got)
	}
	if got := answered(rep1, inverse); got[limited] || !got[open] {
		t.Errorf("the meetings behind Rep1's notes answered %v; want the open meeting and not the limited one", got)
	}
}
