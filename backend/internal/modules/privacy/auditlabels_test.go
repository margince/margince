// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordingLabeler stands in for the compose seam, which is a boundary to this
// module, and remembers every question it was asked.
type recordingLabeler struct {
	names  map[ids.UUID]string
	failOn string
	asked  map[string][][]ids.UUID
}

func (l *recordingLabeler) Labels(_ context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	l.asked[entityType] = append(l.asked[entityType], want)
	if entityType == l.failOn {
		return nil, errors.New("the team read is down")
	}
	out := map[ids.UUID]string{}
	for _, id := range want {
		if name, ok := l.names[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

func entryAbout(entityType string, id ids.UUID) AuditEntry {
	return AuditEntry{ID: ids.NewV7(), EntityType: entityType, EntityID: &id}
}

func labelOf(e AuditEntry) string {
	if e.EntityLabel == nil {
		return "<null>"
	}
	return *e.EntityLabel
}

func TestAPageAsksOncePerEntityTypeWithEachRecordOnce(t *testing.T) {
	weber, dana, deal, team := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	labeler := &recordingLabeler{
		names:  map[ids.UUID]string{weber: "Weber GmbH", dana: "Dana Weiss", deal: "Fleet retrofit", team: "Field"},
		failOn: "team",
		asked:  map[string][][]ids.UUID{},
	}
	entries := []AuditEntry{
		entryAbout("contact", dana),
		entryAbout("company", weber),
		entryAbout("contact", dana),
		entryAbout("deal", deal),
		entryAbout("team", team),
		entryAbout("contact", weber),
		{ID: ids.NewV7(), EntityType: "contact"},
	}

	labelAuditPage(t.Context(), labeler, entries)

	for entityType, calls := range labeler.asked {
		if len(calls) != 1 {
			t.Fatalf("%s was asked %d times for one page, want once", entityType, len(calls))
		}
	}
	if got := slices.Sorted(maps.Keys(labeler.asked)); !slices.Equal(got, []string{"company", "contact", "deal", "team"}) {
		t.Fatalf("types asked = %v, want exactly the four on the page", got)
	}
	if got := labeler.asked["contact"][0]; len(got) != 2 {
		t.Fatalf("contacts asked = %v, want the two distinct ids once each", got)
	}

	want := []string{"Dana Weiss", "Weber GmbH", "Dana Weiss", "Fleet retrofit", "<null>", "Weber GmbH", "<null>"}
	for i, e := range entries {
		if labelOf(e) != want[i] {
			t.Errorf("entry %d (%s) label = %s, want %s", i, e.EntityType, labelOf(e), want[i])
		}
	}
}

// The same id is one record under one type and a different one under another.
// A label must never cross from one type's answer to another type's row.
func TestALabelStaysWithTheTypeThatAnsweredIt(t *testing.T) {
	shared := ids.NewV7()
	labeler := &recordingLabeler{names: map[ids.UUID]string{shared: "Weber GmbH"}, failOn: "contact", asked: map[string][][]ids.UUID{}}
	entries := []AuditEntry{entryAbout("company", shared), entryAbout("contact", shared)}

	labelAuditPage(t.Context(), labeler, entries)

	if labelOf(entries[0]) != "Weber GmbH" || labelOf(entries[1]) != "<null>" {
		t.Fatalf("labels = %s, %s; want the company named and the failed contact type null",
			labelOf(entries[0]), labelOf(entries[1]))
	}
}

func TestAnEmptyNameReadsNullNotBlank(t *testing.T) {
	id := ids.NewV7()
	labeler := &recordingLabeler{names: map[ids.UUID]string{id: ""}, asked: map[string][][]ids.UUID{}}
	entries := []AuditEntry{entryAbout("user", id)}

	labelAuditPage(t.Context(), labeler, entries)

	if entries[0].EntityLabel != nil {
		t.Fatalf("label = %q for a record with an empty name, want null", *entries[0].EntityLabel)
	}
}

func TestNoLabelerLeavesEveryLabelNull(t *testing.T) {
	entries := []AuditEntry{entryAbout("contact", ids.NewV7())}
	labelAuditPage(t.Context(), nil, entries)
	if entries[0].EntityLabel != nil {
		t.Fatalf("label = %q with no labeler wired, want null", *entries[0].EntityLabel)
	}
}

func TestTheWireCarriesTheLabel(t *testing.T) {
	name := "Weber GmbH"
	e := entryAbout("company", ids.NewV7())
	e.EntityLabel = &name
	out, err := auditEntryToWire(e)
	if err != nil {
		t.Fatalf("rendering an entry: %v", err)
	}
	if out.EntityLabel == nil || *out.EntityLabel != name {
		t.Fatalf("wire entity_label = %v, want %q", out.EntityLabel, name)
	}
}
