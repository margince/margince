// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// The read behind the record page's tag panel.
//
// Every claim here is about permissions or rows, so a stub proves none of it:
// whether a caller outside their row scope gets not-found, whether a caller
// without the vocabulary gets a withheld answer rather than an empty one, and
// what the assignment actually recorded.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type recordTagsBody struct {
	Data []struct {
		TagID      string  `json:"tag_id"`
		Name       string  `json:"name"`
		Archived   bool    `json:"archived"`
		AssignedAt string  `json:"assigned_at"`
		Color      *string `json:"color"`
		AssignedBy *struct {
			UserID      *string `json:"user_id"`
			DisplayName *string `json:"display_name"`
			Kind        string  `json:"kind"`
		} `json:"assigned_by"`
	} `json:"data"`
	Withheld bool `json:"withheld"`
}

func readRecordTags(t *testing.T, e *apptest.AppEnv, entityType, id string) (recordTagsBody, int) {
	t.Helper()
	var body recordTagsBody
	status := e.Call(t, "GET", "/v1/records/"+entityType+"/"+id+"/tags", nil, nil, &body)
	return body, status
}

// One route, three types: the panel that draws them is one component, and a
// per-type block on each record response would be three copies that drift.
func TestRecordTagsAnswersForAllThreeAdvertisedTypes(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Cross Type")

	person := createPersonWithTag(t, e, "Tagged Person", tag)

	var org integration.AnyMap
	if status := e.Call(t, "POST", "/v1/organizations", integration.AnyMap{
		"display_name": "Tagged Company", "source": "ui",
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

	// A deal too: the test name claims three types, and two of them passing
	// says nothing about the third — deal is the one whose row scope differs
	// most from a person's.
	stages := apptest.DiscoverSeededPipeline(t, e)
	var deal integration.AnyMap
	if status := e.Call(t, "POST", "/v1/deals", integration.AnyMap{
		"name": "Tagged Deal", "pipeline_id": stages.PipelineID,
		"stage_id": stages.Open, "source": "manual",
	}, nil, &deal); status != http.StatusCreated {
		t.Fatalf("creating the deal: status=%d body=%v", status, deal)
	}
	dealID, _ := deal["id"].(string)
	var dealTagged integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/apply", integration.AnyMap{
		"entity_type": "deal", "entity_id": dealID,
	}, nil, &dealTagged); status != http.StatusCreated {
		t.Fatalf("tagging the deal: status=%d body=%v", status, dealTagged)
	}

	for _, c := range []struct{ entityType, id string }{
		{"person", person},
		{"organization", orgID},
		{"deal", dealID},
	} {
		body, status := readRecordTags(t, e, c.entityType, c.id)
		if status != http.StatusOK {
			t.Fatalf("%s: status=%d", c.entityType, status)
		}
		if len(body.Data) != 1 || body.Data[0].Name != "Cross Type" {
			t.Errorf("%s carries %+v, want the one tag", c.entityType, body.Data)
		}
		if body.Withheld {
			t.Errorf("%s reads as withheld for a caller who may read the vocabulary", c.entityType)
		}
	}
}

// The assignment says WHO, which is the half the panel shows beside the word.
func TestRecordTagsNamesWhoAppliedTheTag(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Applied By Someone")
	person := createPersonWithTag(t, e, "Tagged", tag)

	body, status := readRecordTags(t, e, "person", person)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if len(body.Data) != 1 {
		t.Fatalf("data = %+v, want one assignment", body.Data)
	}
	got := body.Data[0]
	if got.AssignedBy == nil {
		t.Fatal("the assignment names nobody; the panel cannot say who put the tag there")
	}
	if got.AssignedBy.Kind != "human" {
		t.Errorf("kind = %q, want human — a session applied this", got.AssignedBy.Kind)
	}
	if got.AssignedBy.UserID == nil || *got.AssignedBy.UserID == "" {
		t.Error("the assignment carries no user id")
	}
	if got.AssignedAt == "" {
		t.Error("the assignment carries no instant")
	}
}

// Withheld is NOT empty. A caller who may read the record but not the
// vocabulary has to be distinguishable from one looking at a record with no
// tags, because "no tags" is a claim about the record they cannot make.
func TestRecordTagsWithheldIsNotTheSameAsEmpty(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Hidden Word")
	tagged := createPersonWithTag(t, e, "Has A Tag", tag)
	untagged := createPersonWithTag(t, e, "Has No Tags")

	// The genuinely empty record first, while the caller still holds the
	// vocabulary: it has to read as NOT withheld, which is the distinction the
	// flag exists to carry.
	emptyBody, status := readRecordTags(t, e, "person", untagged)
	if status != http.StatusOK {
		t.Fatalf("reading the untagged person: status=%d", status)
	}
	if emptyBody.Withheld {
		t.Error("a record with no tags reads as withheld; the panel would say the wrong thing")
	}
	if len(emptyBody.Data) != 0 {
		t.Errorf("the untagged person carries %+v", emptyBody.Data)
	}

	// Now take the vocabulary away and read the TAGGED record. Revoking on the
	// role document is what the product itself reads, so this is the same
	// refusal a rep in a workspace without the grant would meet.
	revokeTagRead(t, e)

	withheldBody, status := readRecordTags(t, e, "person", tagged)
	if status != http.StatusOK {
		t.Fatalf("reading as a caller without the vocabulary: status=%d", status)
	}
	if !withheldBody.Withheld {
		t.Error("a caller without tag.read did not get withheld=true")
	}
	if len(withheldBody.Data) != 0 {
		t.Errorf("withheld answer carries %d assignments, want none", len(withheldBody.Data))
	}
}

// revokeTagRead drops tag.read from every system role in the workspace, so the
// session's own caller loses the vocabulary.
func revokeTagRead(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE role SET permissions = jsonb_set(
			permissions, '{objects,tag}',
			'{"create": false, "read": false, "update": false, "delete": false}'::jsonb, true)
		 WHERE is_system`); err != nil {
		t.Fatalf("revoking tag.read: %v", err)
	}
}

// A retired word stays ON the record — archiving stops a tag being applied, it
// does not un-tag a history that was true — and rides marked so the panel can
// mute it rather than hide it.
func TestRecordTagsCarriesArchivedWordsMarked(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Retired Word")
	person := createPersonWithTag(t, e, "Tagged Before Retirement", tag)

	var archived integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+tag, nil, nil, &archived); status != http.StatusOK {
		t.Fatalf("archiving: status=%d body=%v", status, archived)
	}

	body, status := readRecordTags(t, e, "person", person)
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if len(body.Data) != 1 {
		t.Fatalf("data = %+v, want the retired word still on the record", body.Data)
	}
	if !body.Data[0].Archived {
		t.Error("the retired word does not ride marked; the panel would draw it as current")
	}
}

// The three advertised types only. `taggable` admits lead and project, and
// answering for them here would ship a surface no screen offers.
func TestRecordTagsRefusesATypeItDoesNotServe(t *testing.T) {
	e := tagEnv(t)
	var problem integration.AnyMap
	if status := e.Call(t, "GET", "/v1/records/lead/"+ids.NewV7().String()+"/tags", nil, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("reading a lead's tags: status=%d body=%v, want 422", status, problem)
	}
}
