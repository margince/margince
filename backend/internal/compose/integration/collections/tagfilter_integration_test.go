// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// Filtering a list by tag, in the three modes, over the composed server.
//
// The fixture is the smallest one where the modes give three different
// answers: two tags and three people, one carrying each tag and one carrying
// both. A fixture where any two modes agree proves only that a filter ran.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// taggedTrio seeds A-only, A-and-B, and B-only, and answers the two tag ids.
func taggedTrio(t *testing.T, e *apptest.AppEnv) (tagA, tagB string) {
	t.Helper()
	tagA = createTag(t, e, "Filter A")
	tagB = createTag(t, e, "Filter B")
	createPersonWithTag(t, e, "Carries A", tagA)
	createPersonWithTag(t, e, "Carries Both", tagA, tagB)
	createPersonWithTag(t, e, "Carries B", tagB)
	return tagA, tagB
}

// listPeopleNames answers the names one query selects.
func listPeopleNames(t *testing.T, e *apptest.AppEnv, query string) []string {
	t.Helper()
	var page struct {
		Data []struct {
			FullName string `json:"full_name"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/people?"+query, nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing people with %q: status=%d", query, status)
	}
	out := make([]string, 0, len(page.Data))
	for _, p := range page.Data {
		out = append(out, p.FullName)
	}
	return out
}

func TestTagModeAnySelectsARecordCarryingEitherTag(t *testing.T) {
	e := tagEnv(t)
	tagA, tagB := taggedTrio(t, e)

	got := listPeopleNames(t, e, "tag_id="+tagA+"&tag_id="+tagB+"&tag_mode=any")
	if len(got) != 3 {
		t.Errorf("any selected %v, want all three — each carries at least one", got)
	}
}

func TestTagModeAllSelectsOnlyARecordCarryingEveryTag(t *testing.T) {
	e := tagEnv(t)
	tagA, tagB := taggedTrio(t, e)

	got := listPeopleNames(t, e, "tag_id="+tagA+"&tag_id="+tagB+"&tag_mode=all")
	if len(got) != 1 || got[0] != "Carries Both" {
		t.Errorf("all selected %v, want only the person carrying both", got)
	}
}

func TestTagModeNoneSelectsARecordCarryingNeitherTag(t *testing.T) {
	e := tagEnv(t)
	tagA, _ := taggedTrio(t, e)

	// Only tag A is named, so `none` keeps the person carrying B alone. The
	// person carrying both is excluded because they carry A.
	got := listPeopleNames(t, e, "tag_id="+tagA+"&tag_mode=none")
	if len(got) != 1 || got[0] != "Carries B" {
		t.Errorf("none selected %v, want only the person carrying neither", got)
	}
}

// A record carrying NO tags at all is selected by `none` — it carries not one
// of the named words, which is what the mode says. A NOT EXISTS that had been
// written as a negation inside the subquery would drop it.
func TestTagModeNoneSelectsARecordWithNoTagsAtAll(t *testing.T) {
	e := tagEnv(t)
	tagA, _ := taggedTrio(t, e)
	createPersonWithTag(t, e, "Carries Nothing")

	got := listPeopleNames(t, e, "tag_id="+tagA+"&tag_mode=none")
	var found bool
	for _, name := range got {
		if name == "Carries Nothing" {
			found = true
		}
	}
	if !found {
		t.Errorf("none selected %v, which omits the untagged person — they carry not one of the named tags", got)
	}
}

// An archived tag selects nothing. It is not in the picker, so a filter naming
// it describes a slice no reader can construct, and after a merge releases a
// name a re-coined word would otherwise drag the old tag's records along.
func TestFilteringByAnArchivedTagSelectsNothing(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Retired Filter")
	createPersonWithTag(t, e, "Tagged Before Retirement", tag)

	var archived integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+tag, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("archiving: status=%d body=%v", status, archived)
	}

	if got := listPeopleNames(t, e, "tag_id="+tag+"&tag_mode=any"); len(got) != 0 {
		t.Errorf("filtering by a retired tag selected %v, want nothing", got)
	}
}

// A mode the enum does not admit is refused, not defaulted. Treating a typo as
// `any` would hand back a wider slice than the caller asked for.
func TestAnUnknownTagModeIsRefused(t *testing.T) {
	e := tagEnv(t)
	tagA, _ := taggedTrio(t, e)

	var problem integration.AnyMap
	if status := e.Call(t, "GET", "/v1/people?tag_id="+tagA+"&tag_mode=most", nil, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("an unknown tag_mode: status=%d body=%v, want 422", status, problem)
	}
}

// The mode alone is not a filter: with no tag named there is nothing to
// combine, and every record stays in the page.
func TestATagModeWithoutTagsFiltersNothing(t *testing.T) {
	e := tagEnv(t)
	taggedTrio(t, e)

	got := listPeopleNames(t, e, "tag_mode=all")
	if len(got) != 3 {
		t.Errorf("a mode with no tags selected %v, want every person", got)
	}
}

// A list ROW carries its tags, so a table can draw a chip without a second
// read per row. Filtering by tag and showing them are different features, and
// shipping the filter alone hides tags on exactly the screens where they
// explain why a row is in the list.
func TestAListRowCarriesItsOwnTags(t *testing.T) {
	e := tagEnv(t)
	tagA, _ := taggedTrio(t, e)

	var page struct {
		Data []struct {
			FullName string `json:"full_name"`
			Tags     []struct {
				TagID string `json:"tag_id"`
				Name  string `json:"name"`
			} `json:"tags"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/people?tag_id="+tagA, nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing: status=%d", status)
	}
	if len(page.Data) != 2 {
		t.Fatalf("selected %d people, want the two carrying tag A", len(page.Data))
	}
	for _, row := range page.Data {
		if len(row.Tags) == 0 {
			t.Errorf("%s carries no tags on the row; the table has nothing to draw", row.FullName)
			continue
		}
		var carriesA bool
		for _, tag := range row.Tags {
			if tag.TagID == tagA {
				carriesA = true
			}
		}
		if !carriesA {
			t.Errorf("%s's row tags %+v omit the tag it was selected by", row.FullName, row.Tags)
		}
	}

	// The row carrying BOTH reports both: a cap that dropped one would make a
	// chip strip say less than the record holds.
	for _, row := range page.Data {
		if row.FullName == "Carries Both" && len(row.Tags) != 2 {
			t.Errorf("the person carrying two tags reports %d on the row", len(row.Tags))
		}
	}
}

// An archived tag is not drawn on a list. A retired word is not in the picker,
// so a chip for it sends a reader somewhere they cannot go.
func TestAListRowOmitsArchivedTags(t *testing.T) {
	e := tagEnv(t)
	live := createTag(t, e, "Still Live")
	retired := createTag(t, e, "Since Retired")
	createPersonWithTag(t, e, "Carries Both Kinds", live, retired)

	var archived integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+retired, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("archiving: status=%d body=%v", status, archived)
	}

	var page struct {
		Data []struct {
			Tags []struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/people?tag_id="+live, nil, nil, &page); status != http.StatusOK {
		t.Fatalf("listing: status=%d", status)
	}
	if len(page.Data) != 1 {
		t.Fatalf("selected %d people, want the one", len(page.Data))
	}
	for _, tag := range page.Data[0].Tags {
		if tag.Name == "Since Retired" {
			t.Error("the row draws a retired word; the picker no longer offers it")
		}
	}
}
