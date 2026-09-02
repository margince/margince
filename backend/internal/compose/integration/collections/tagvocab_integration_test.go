// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// The vocabulary-management surface over the composed server: read a word with
// its weight, rename it, restore it, fold two into one.
//
// These need a real database because every claim they make is about rows —
// which taggings moved, which collapsed, what the uniqueness index refuses.
// A store test with a stub would be asserting its own arithmetic.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func tagEnv(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithSchemaPool(integration.SchemaPool(t)))
	apptest.BootstrapWorkspaceSession(t, e, "Tag Vocabulary", "admin@tagvocab.test", "Admin")
	return e
}

// createTag makes one word and answers its id.
func createTag(t *testing.T, e *apptest.AppEnv, name string) string {
	t.Helper()
	var tag integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags", integration.AnyMap{"name": name}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("creating the tag %q: status=%d body=%v", name, status, tag)
	}
	id, ok := tag["id"].(string)
	if !ok || id == "" {
		t.Fatalf("the created tag carries no id: %v", tag)
	}
	return id
}

// createPersonWithTag makes a person and hangs the named tags on them.
func createPersonWithTag(t *testing.T, e *apptest.AppEnv, name string, tagIDs ...string) string {
	t.Helper()
	var person integration.AnyMap
	if status := e.Call(t, "POST", "/v1/people", integration.AnyMap{
		"full_name": name, "source": "ui",
	}, nil, &person); status != http.StatusCreated {
		t.Fatalf("creating %q: status=%d body=%v", name, status, person)
	}
	id, _ := person["id"].(string)
	for _, tagID := range tagIDs {
		var applied integration.AnyMap
		if status := e.Call(t, "POST", "/v1/tags/"+tagID+"/apply", integration.AnyMap{
			"entity_type": "person", "entity_id": id,
		}, nil, &applied); status != http.StatusCreated {
			t.Fatalf("tagging %q: status=%d body=%v", name, status, applied)
		}
	}
	return id
}

// Merge separates MOVED from COLLAPSED because they are different facts, and a
// single total would tell an admin the wrong number. The fixture is built so
// the two differ: one person carries only the source, one carries both.
func TestMergeCountsWhatMovedApartFromWhatCollapsed(t *testing.T) {
	e := tagEnv(t)
	source := createTag(t, e, "Keyaccount")
	target := createTag(t, e, "Key Account")

	createPersonWithTag(t, e, "Only Source", source)
	createPersonWithTag(t, e, "Carries Both", source, target)

	var result struct {
		IntoTagID string `json:"into_tag_id"`
		Moved     int    `json:"moved"`
		Collapsed int    `json:"collapsed"`
	}
	if status := e.Call(t, "POST", "/v1/tags/"+source+"/merge", integration.AnyMap{
		"into_tag_id": target,
	}, nil, &result); status != http.StatusOK {
		t.Fatalf("merging: status=%d body=%+v", status, result)
	}

	if result.Moved != 1 {
		t.Errorf("moved = %d, want 1 — only the person carrying the source alone moves", result.Moved)
	}
	if result.Collapsed != 1 {
		t.Errorf("collapsed = %d, want 1 — the person already carrying both gains nothing", result.Collapsed)
	}

	// The target's own weight is what the numbers claimed: it had one person
	// and gained exactly the moved one.
	var detail struct {
		Usage struct{ People int } `json:"usage"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+target, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading the target: status=%d", status)
	}
	if detail.Usage.People != 2 {
		t.Errorf("the target carries %d people, want 2 — one it had, one that moved", detail.Usage.People)
	}

	// And the collapsed row is GONE, not merely uncounted. A merge that left
	// the duplicate behind would report the same numbers and leave the person
	// carrying a tag nobody can see.
	var srcDetail detail0
	if status := e.Call(t, "GET", "/v1/tags/"+source, nil, nil, &srcDetail); status != http.StatusOK {
		t.Fatalf("reading the merged source: status=%d", status)
	}
	if srcDetail.Usage.People != 0 {
		t.Errorf("the merged-away tag still carries %d people; every tagging was meant to move or be dropped", srcDetail.Usage.People)
	}
}

// detail0 is the shape both merge tests read back.
type detail0 struct {
	Usage struct{ People int } `json:"usage"`
}

// The source is archived and its NAME IS RELEASED. That is the product
// decision and it has a cost: somebody can coin the same word again. Asserting
// the reuse succeeds is what stops a future change quietly reintroducing an
// alias nobody asked for.
func TestMergeArchivesTheSourceAndReleasesItsName(t *testing.T) {
	e := tagEnv(t)
	source := createTag(t, e, "Churnrisk")
	target := createTag(t, e, "Churn Risk")

	var result integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+source+"/merge", integration.AnyMap{
		"into_tag_id": target,
	}, nil, &result); status != http.StatusOK {
		t.Fatalf("merging: status=%d body=%v", status, result)
	}

	var archived struct {
		ArchivedAt *string `json:"archived_at"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+source, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("reading the merged source: status=%d", status)
	}
	if archived.ArchivedAt == nil {
		t.Error("the merged source is not archived; it would still be applicable")
	}

	// The released name is free. A workspace that kept it as an alias would
	// refuse this with a conflict.
	var reborn integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags", integration.AnyMap{"name": "Churnrisk"}, nil, &reborn); status != http.StatusCreated {
		t.Errorf("recreating the merged-away name: status=%d body=%v — the name was meant to be released", status, reborn)
	}
}

// Merging a tag into itself would archive the only survivor and take every
// tagging with it. It is refused at the door, naming the field that carried it.
func TestMergeRefusesATagIntoItself(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Inbound")
	createPersonWithTag(t, e, "Tagged", tag)

	var problem integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/merge", integration.AnyMap{
		"into_tag_id": tag,
	}, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("merging a tag into itself: status=%d body=%v, want 422", status, problem)
	}

	// And it is untouched — a refusal that had already archived the tag would
	// be worse than the merge it refused.
	var detail struct {
		ArchivedAt *string              `json:"archived_at"`
		Usage      struct{ People int } `json:"usage"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+tag, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading it back: status=%d", status)
	}
	if detail.ArchivedAt != nil || detail.Usage.People != 1 {
		t.Errorf("the refused merge changed the tag: archived=%v people=%d", detail.ArchivedAt, detail.Usage.People)
	}
}

// Restoring is the undo for a retirement, and it refuses when a live tag has
// taken the name meanwhile — two words a reader cannot tell apart is the state
// the vocabulary exists to prevent.
func TestRestoreRefusesWhenALiveTagHasTakenTheName(t *testing.T) {
	e := tagEnv(t)
	original := createTag(t, e, "Parked")

	var archived integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+original, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("archiving: status=%d body=%v", status, archived)
	}

	// Somebody coins the name again while the first is retired. This is only
	// possible because uq_tag_name binds LIVE rows: an archived word no longer
	// reserves its name, which is the same rule that lets a merge release one.
	createTag(t, e, "Parked")

	var problem integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+original+"/restore", nil, nil, &problem); status != http.StatusConflict {
		t.Fatalf("restoring onto a taken name: status=%d body=%v, want 409", status, problem)
	}
}

func TestRestoreBringsBackAWordNobodyElseTook(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Trade Fair 2026")
	createPersonWithTag(t, e, "Met At The Fair", tag)

	var archived integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+tag, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("archiving: status=%d body=%v", status, archived)
	}

	var restored struct {
		ArchivedAt *string `json:"archived_at"`
	}
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/restore", nil, nil, &restored); status != http.StatusOK {
		t.Fatalf("restoring: status=%d", status)
	}
	if restored.ArchivedAt != nil {
		t.Error("the restored tag still reads as archived")
	}

	// Archiving never dropped the taggings, so the weight comes back with it.
	var detail struct {
		Usage struct{ People int } `json:"usage"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+tag, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading it back: status=%d", status)
	}
	if detail.Usage.People != 1 {
		t.Errorf("the restored tag carries %d people, want 1 — archiving retires a word, it does not untag records", detail.Usage.People)
	}
}

// A rename onto a name another live tag holds is a conflict, not a silent
// second row: the uniqueness index folds case, and so must the refusal.
func TestRenamingOntoALiveNameIsRefused(t *testing.T) {
	e := tagEnv(t)
	first := createTag(t, e, "Champion")
	createTag(t, e, "Detractor")

	var problem integration.AnyMap
	if status := e.Call(t, "PATCH", "/v1/tags/"+first, integration.AnyMap{
		"name": "detractor",
	}, nil, &problem); status != http.StatusConflict {
		t.Fatalf("renaming onto a taken name: status=%d body=%v, want 409", status, problem)
	}
}

// The usage counts cover the three advertised types and stop there. Lead and
// project taggings exist in storage; counting them would report a weight no
// screen in V1 can explain.
func TestUsageCountsOnlyTheAdvertisedRecordTypes(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Cross Type")
	createPersonWithTag(t, e, "A Person", tag)

	var org integration.AnyMap
	if status := e.Call(t, "POST", "/v1/organizations", integration.AnyMap{
		"display_name": "A Company", "source": "ui",
	}, nil, &org); status != http.StatusCreated {
		t.Fatalf("creating the company: status=%d body=%v", status, org)
	}
	orgID, _ := org["id"].(string)
	var applied integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/apply", integration.AnyMap{
		"entity_type": "organization", "entity_id": orgID,
	}, nil, &applied); status != http.StatusCreated {
		t.Fatalf("tagging the company: status=%d body=%v", status, applied)
	}

	var detail struct {
		Usage struct {
			People    int `json:"people"`
			Companies int `json:"companies"`
			Deals     int `json:"deals"`
		} `json:"usage"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+tag, nil, nil, &detail); status != http.StatusOK {
		t.Fatalf("reading the tag: status=%d", status)
	}
	if detail.Usage.People != 1 || detail.Usage.Companies != 1 || detail.Usage.Deals != 0 {
		t.Errorf("usage = %+v, want 1 person, 1 company, 0 deals", detail.Usage)
	}
}

// The rename path itself, which the conflict test above cannot reach: a
// handler that always answered 409 would pass that one.
func TestRenamingRecolouringAndDescribingATag(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Draft Name")

	var updated struct {
		Name        string  `json:"name"`
		Color       *string `json:"color"`
		Description *string `json:"description"`
	}
	if status := e.Call(t, "PATCH", "/v1/tags/"+tag, integration.AnyMap{
		"name": "Settled Name", "color": "teal", "description": "What it means",
	}, nil, &updated); status != http.StatusOK {
		t.Fatalf("renaming: status=%d body=%+v", status, updated)
	}
	if updated.Name != "Settled Name" {
		t.Errorf("name = %q, want the new one", updated.Name)
	}
	if updated.Color == nil || *updated.Color != "teal" {
		t.Errorf("color = %v, want teal", updated.Color)
	}
	if updated.Description == nil || *updated.Description != "What it means" {
		t.Errorf("description = %v, want the text sent", updated.Description)
	}

	// Clearing is a VALUE, not a null: the generated request type cannot tell
	// an absent field from a null one, so "none" and "" are what carry it.
	var cleared struct {
		Name        string  `json:"name"`
		Color       *string `json:"color"`
		Description *string `json:"description"`
	}
	if status := e.Call(t, "PATCH", "/v1/tags/"+tag, integration.AnyMap{
		"color": "none", "description": "",
	}, nil, &cleared); status != http.StatusOK {
		t.Fatalf("clearing: status=%d body=%+v", status, cleared)
	}
	if cleared.Color != nil {
		t.Errorf("color = %v after clearing, want absent", *cleared.Color)
	}
	if cleared.Description != nil && *cleared.Description != "" {
		t.Errorf("description = %v after clearing, want absent", *cleared.Description)
	}
	// The name was not mentioned in the clearing call and must survive it.
	if cleared.Name != "Settled Name" {
		t.Errorf("name = %q after a colour-only patch; an omitted field must be left alone", cleared.Name)
	}
}

// A stale If-Match is version skew, and the write must not land.
func TestAStaleIfMatchRefusesTheRename(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Versioned")

	var problem integration.AnyMap
	if status := e.Call(t, "PATCH", "/v1/tags/"+tag, integration.AnyMap{"name": "Renamed"},
		map[string]string{"If-Match": "99"}, &problem); status != http.StatusConflict {
		t.Fatalf("a stale If-Match: status=%d body=%v, want 409", status, problem)
	}

	var after struct {
		Name string `json:"name"`
	}
	if status := e.Call(t, "GET", "/v1/tags/"+tag, nil, nil, &after); status != http.StatusOK {
		t.Fatalf("reading it back: status=%d", status)
	}
	if after.Name != "Versioned" {
		t.Errorf("name = %q; the refused rename was applied anyway", after.Name)
	}
}
