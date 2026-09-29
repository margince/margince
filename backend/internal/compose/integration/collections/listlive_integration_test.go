// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// A Live List is its filter, evaluated completely and now: past a thousand
// matches, with a date counted back from today, and explained record by record
// by the same SQL that decides who is on it.

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	collectionsmod "github.com/margince/margince/backend/internal/modules/collections"
	contactsmod "github.com/margince/margince/backend/internal/modules/contacts"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// liveList makes a Live List on contact over definition.
func (f fixture) liveList(t *testing.T, name string, definition map[string]any) ids.ListID {
	t.Helper()
	created, err := f.lists.CreateList(f.ctx, collectionsmod.CreateListInput{
		Name: name, EntityType: "contact", ListType: "dynamic", Definition: definition,
	})
	if err != nil {
		t.Fatalf("create list %q: %v", name, err)
	}
	return created.ID
}

// everyMember pages through a list's members and answers them all.
func (f fixture) everyMember(t *testing.T, list ids.ListID, pageSize int) map[ids.UUID]bool {
	t.Helper()
	out := map[ids.UUID]bool{}
	cursor := ""
	for {
		rows, page, err := f.lists.ListMembers(f.ctx, list, pageSize, cursor)
		if err != nil {
			t.Fatalf("list members: %v", err)
		}
		for _, row := range rows {
			if out[row.EntityID] {
				t.Fatalf("member %s answered twice while paging", row.EntityID)
			}
			out[row.EntityID] = true
		}
		if !page.HasMore {
			return out
		}
		cursor = page.NextCursor
	}
}

// previewCount is what the filter builder's preview counts for the same
// definition: the engine's own count, the one /filters/preview shows.
func (f fixture) previewCount(t *testing.T, definition map[string]any) int {
	t.Helper()
	engine, _, err := f.lists.SegmentEngine(f.ctx, "contact")
	if err != nil {
		t.Fatal(err)
	}
	pred, err := collectionsmod.PredicateFromDefinition(definition)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.e.DB().Tx(f.ctx, func(tx pgx.Tx) error {
		n, err = engine.CountMatching(f.ctx, tx, pred)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestALiveListPastAThousandMatchesIsCompleteAndEqualsItsPreview(t *testing.T) {
	f := setupFixture(t)
	column := f.defineField(t, customfieldsmod.FieldSpec{Label: "Cohort Big", Type: customfieldsmod.TypeText})
	const matching = 1010
	for i := range matching + 5 {
		cohort := "k5"
		if i >= matching {
			cohort = "other"
		}
		if _, err := f.contacts.CreateContact(f.ctx, contactsmod.CreateContactInput{
			FullName: fmt.Sprintf("Cohort %04d", i), Source: "manual", CustomFields: map[string]any{column: cohort},
		}); err != nil {
			t.Fatalf("seed contact %d: %v", i, err)
		}
	}
	definition := map[string]any{"field": column, "op": "eq", "value": "k5"}
	list := f.liveList(t, "Every K5", definition)

	members := f.everyMember(t, list, 200)
	if len(members) != matching {
		t.Fatalf("paged %d members, want every one of the %d matches — a cap would stop at %d",
			len(members), matching, storekit.PredicateRowLimit)
	}
	if preview := f.previewCount(t, definition); preview != matching {
		t.Fatalf("the preview counts %d for the same filter, the list pages %d", preview, len(members))
	}
	count, err := f.lists.CountMembers(f.ctx, list)
	if err != nil || count != matching {
		t.Fatalf("CountMembers = %d, %v, want %d", count, err, matching)
	}
}

func TestARelativeDateSelectsAgainstToday(t *testing.T) {
	f := setupFixture(t)
	column := f.defineField(t, customfieldsmod.FieldSpec{Label: "Last Touch Rel", Type: customfieldsmod.TypeDate})
	recent, stale := f.seedTwoContacts(t, "Touch")
	never, _ := f.seedTwoContacts(t, "Untouched")
	today := time.Now().UTC()
	f.setField(t, recent, column, today.AddDate(0, 0, -10).Format(time.DateOnly))
	f.setField(t, stale, column, today.AddDate(0, 0, -60).Format(time.DateOnly))

	within := map[string]any{"field": column, "op": "gte", "value": map[string]any{"days_ago": 45}}
	longAgo := map[string]any{"field": column, "op": "lt", "value": map[string]any{"days_ago": 45}}

	assertSoleMember(t, f, f.liveList(t, "Touched in 45 days", within), recent)
	assertSoleMember(t, f, f.liveList(t, "Quiet for 45 days", longAgo), stale)
	if got := f.previewCount(t, longAgo); got != 1 {
		t.Fatalf("the preview counts %d for 'more than 45 days ago', want 1 (the contact never touched is neither)", got)
	}
	members := f.everyMember(t, f.liveList(t, "Quiet or never", map[string]any{"or": []any{
		longAgo, map[string]any{"field": column, "op": "exists", "value": false},
	}}), 50)
	if !members[stale] || !members[never] || members[recent] {
		t.Fatalf("members = %v, want the stale and the untouched contact", members)
	}
}

func TestExplainAgreesWithMembershipForEveryRecord(t *testing.T) {
	f := setupFixture(t)
	tier := f.defineField(t, customfieldsmod.FieldSpec{Label: "Explain Tier", Type: customfieldsmod.TypeText})
	score := f.defineField(t, customfieldsmod.FieldSpec{Label: "Explain Score", Type: customfieldsmod.TypeNumber})
	tag := f.tagNamed(t, "explain-vip")

	var seeded []ids.UUID
	for i, values := range []map[string]any{
		{tier: "gold", score: 80.0},
		{tier: "gold"},
		{tier: "silver", score: 95.0},
		{score: 10.0},
		{},
	} {
		c, err := f.contacts.CreateContact(f.ctx, contactsmod.CreateContactInput{
			FullName: fmt.Sprintf("Explain %d", i), Source: "manual", CustomFields: values,
		})
		if err != nil {
			t.Fatal(err)
		}
		seeded = append(seeded, ids.UUID(c.Id))
	}
	f.applyTag(t, tag, seeded[3])

	definition := map[string]any{"or": []any{
		map[string]any{"and": []any{
			map[string]any{"field": tier, "op": "eq", "value": "gold"},
			map[string]any{"field": score, "op": "gt", "value": 50.0},
		}},
		map[string]any{"field": "tag", "op": "eq", "value": tag.String()},
		map[string]any{"field": score, "op": "gte", "value": 90.0},
	}}
	list := f.liveList(t, "Explained", definition)
	members := f.everyMember(t, list, 50)
	for _, id := range seeded {
		why, err := f.lists.ExplainMember(f.ctx, list, id)
		if err != nil {
			t.Fatalf("explain %s: %v", id, err)
		}
		if why.Member != members[id] {
			t.Fatalf("explain says member=%v for %s, membership says %v", why.Member, id, members[id])
		}
	}
	if len(members) != 3 {
		t.Fatalf("members = %d, want the gold 80, the tagged and the silver 95", len(members))
	}
	why, err := f.lists.ExplainMember(f.ctx, list, seeded[1])
	if err != nil {
		t.Fatal(err)
	}
	gold80 := why.Clauses.Children[0]
	if gold80.Result != nil || gold80.Children[1].Result != nil || gold80.Children[0].Value == nil {
		t.Fatalf("gold with no score: the AND and its score clause must answer unknown and the tier its value, got %+v", gold80)
	}
}

func TestAMaskedFieldIsHiddenInTheWhy(t *testing.T) {
	f := setupFixture(t)
	secret := f.defineField(t, customfieldsmod.FieldSpec{Label: "Masked Margin", Type: customfieldsmod.TypeText})
	target, _ := f.seedTwoContacts(t, "Masked")
	f.setField(t, target, secret, "thin")
	list := f.liveList(t, "Thin margin", map[string]any{"field": secret, "op": "eq", "value": "thin"})

	// Team scope, because a reader who sees every row is never masked.
	masked := testPerms
	masked.RowScope = principal.RowScopeTeam
	masked.FieldMasks = []principal.FieldMask{{Object: "contact", Field: secret, Condition: principal.MaskAlways}}
	reader := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, masked)
	why, err := f.lists.ExplainMember(reader, list, target)
	if err != nil {
		t.Fatal(err)
	}
	// Not a member for this reader: a masked clause selects nothing, or the
	// list's membership would name who carries the value the mask withholds.
	if why.Member || !why.Clauses.Hidden || why.Clauses.Value != nil {
		t.Fatalf("a masked reader was shown member=%v %+v, want no member and the value hidden", why.Member, why.Clauses)
	}
	open, err := f.lists.ExplainMember(f.ctx, list, target)
	if err != nil {
		t.Fatal(err)
	}
	if open.Clauses.Hidden || open.Clauses.Value == nil || *open.Clauses.Value != "thin" {
		t.Fatalf("an unmasked reader was shown %+v, want the value", open.Clauses)
	}
}

// tagNamed coins a tag through the tag writer and answers its id.
func (f fixture) tagNamed(t *testing.T, name string) ids.TagID {
	t.Helper()
	tag, err := f.lists.CreateTag(f.ctx, name, nil, nil)
	if err != nil {
		t.Fatalf("create tag %q: %v", name, err)
	}
	return tag.ID
}

// applyTag applies a tag to a contact through the tag writer.
func (f fixture) applyTag(t *testing.T, tag ids.TagID, contact ids.UUID) {
	t.Helper()
	if _, err := f.lists.ApplyTag(f.ctx, tag, "contact", contact); err != nil {
		t.Fatalf("apply tag: %v", err)
	}
}
