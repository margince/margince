// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUnchangedTechnicalRefreshIsNotCompletedWork(t *testing.T) {
	t.Parallel()
	row := entry{
		Action: actionUpdate, EntityType: typeCompany,
		Evidence: []byte(`{"facts":[{"field":"mail_provider","value":"Example Mail"}],"technical":{"changes":0}}`),
	}
	if _, shown := describe(row); shown {
		t.Fatal("a background refresh without changes became completed work")
	}
}

func TestTechnicalReceiptsShowOnlyActualChanges(t *testing.T) {
	t.Parallel()
	row := entry{Action: actionUpdate, EntityType: typeCompany, Evidence: []byte(`{
		"facts":[{"field":"email_security","value":"Unchanged SPF"}],
		"technical":{"changes":2,"field_changes":[
			{"field":"mail_provider","before":"Old Mail","after":"New Mail"},
			{"field":"operated_service","before":"Shop","after":""}
		]}}`)}
	if _, shown := describe(row); !shown {
		t.Fatal("a technical change disappeared from completed work")
	}
	changes := changesOf(row)
	if len(changes) != 2 || changes[0].Before != "Old Mail" || changes[0].After != "New Mail" ||
		changes[1].Before != "Shop" || changes[1].After != "" {
		t.Fatalf("receipt did not show the recorded changes: %+v", changes)
	}
}

func TestLegacyTechnicalEvidenceDoesNotInventPreviousValues(t *testing.T) {
	t.Parallel()
	row := entry{Action: actionUpdate, Evidence: []byte(`{"facts":[{"field":"mail_provider","value":"New Mail"}],"technical":{"changes":1}}`)}
	if changes := changesOf(row); len(changes) != 0 {
		t.Fatalf("unrecorded prior values were fabricated: %+v", changes)
	}
}

func TestTechnicalReceiptCountsAndDrawerExcludeUnchangedRefreshes(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.June, 12, 10, 0, 0, 0, time.UTC)
	changed := auditRow("agent:technical-lookup", "", "", "", at)
	changed.EntityType = typeCompany
	changed.Evidence = []byte(`{"technical":{"changes":1}}`)
	refresh := changed
	refresh.ID = ids.NewV7()
	refresh.EntityID = ids.NewV7()
	refresh.OccurredAt = at.Add(time.Minute)
	refresh.Evidence = []byte(`{"technical":{"changes":0}}`)
	entries := []entry{refresh, changed}
	lines, hidden := linesOf(unmasked(t), entries, nil, 100)
	if len(lines) != 1 || hidden != 1 || ids.UUID(lines[0].Id) != changed.ID {
		t.Fatalf("refresh changed the receipt summary: lines=%+v hidden=%d", lines, hidden)
	}
	members, found := lineMembers(entries, changed.ID)
	if !found || len(members) != 1 || members[0].ID != changed.ID {
		t.Fatalf("drawer disagrees with receipt: %+v", members)
	}
}
