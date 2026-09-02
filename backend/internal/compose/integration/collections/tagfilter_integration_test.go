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
