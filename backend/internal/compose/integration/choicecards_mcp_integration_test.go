// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the chat cards are drawn from and what their buttons call, on the real
// engines: a create's duplicate pair and tag offer, and the verbs a click sends.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

type createdWithFollowups struct {
	ID                  string `json:"id"`
	DuplicateCandidates []struct {
		CandidateID   string `json:"candidate_id"`
		OtherRecordID string `json:"other_record_id"`
		Evidence      []struct {
			Field string `json:"field"`
			Left  string `json:"left_value"`
			Right string `json:"right_value"`
		} `json:"evidence"`
	} `json:"duplicate_candidates"`
	TagOffer *struct {
		Name      string  `json:"name"`
		TagID     *string `json:"tag_id"`
		Exists    bool    `json:"exists"`
		MayCreate bool    `json:"may_create"`
	} `json:"tag_offer"`
}

// seedAFiledPair creates one contact, then a second that shares only its phone,
// so the second create files the pair and answers with it.
func seedAFiledPair(t *testing.T, invoke func(tool, args string) (string, error), shared string) (first string, second createdWithFollowups) {
	t.Helper()
	first = answered[struct {
		ID string `json:"id"`
	}](t, mustInvoke(t, invoke, "create_record",
		`{"record_type":"contact","fields":{"full_name":"Anna Meyer",`+
			`"phones":[{"phone":"`+shared+`","phone_type":"mobile","is_primary":true}]}}`)).ID
	second = answered[createdWithFollowups](t, mustInvoke(t, invoke, "create_record",
		`{"record_type":"contact","fields":{"full_name":"Anna Meier-Brandt",`+
			`"emails":[{"email":"anna@privat.test","email_type":"personal","is_primary":true}],`+
			`"phones":[{"phone":"`+shared+`","phone_type":"work","is_primary":true}]}}`))
	if len(second.DuplicateCandidates) != 1 {
		t.Fatalf("the second create filed %d pairs, want 1", len(second.DuplicateCandidates))
	}
	return first, second
}

func TestACreateNamesThePairAndTheRecordAlreadyThereForTheCard(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "card reader", "read", "write"))
	first, second := seedAFiledPair(t, invoke, "+4930900000841")

	pair := second.DuplicateCandidates[0]
	if pair.OtherRecordID != first || pair.CandidateID == "" {
		t.Fatalf("pair = %+v, want the record already here (%s) and the queue's pair id", pair, first)
	}
}

func TestACardsMergeFoldsTheCreatedRecordIntoTheOneAlreadyThere(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "card merger", "read", "write"))
	first, second := seedAFiledPair(t, invoke, "+4930900000842")

	out := mustInvoke(t, invoke, "merge_records",
		`{"record_type":"contact","source_id":"`+second.ID+`","target_id":"`+second.DuplicateCandidates[0].OtherRecordID+`"}`)
	merged := answered[struct {
		Merged     bool   `json:"merged"`
		SurvivorID string `json:"survivor_id"`
	}](t, out)
	if !merged.Merged || merged.SurvivorID != first {
		t.Fatalf("merge = %+v, want the record already on file (%s) to survive", merged, first)
	}
}

func TestACardsNotTheSameDismissesThePairAndItsUndoReopensIt(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "card dismisser", "read", "write"))
	_, second := seedAFiledPair(t, invoke, "+4930900000843")
	candidate := second.DuplicateCandidates[0].CandidateID

	dismissedOut := mustInvoke(t, invoke, "decide_duplicate",
		`{"candidate_id":"`+candidate+`","decision":"not_the_same"}`)
	if spec, ok := q.registry.Spec("decide_duplicate"); !ok {
		t.Fatal("decide_duplicate is not registered")
	} else if defect := agents.ResultDefect(spec.OutputSchema, json.RawMessage(dismissedOut)); defect != "" {
		t.Fatalf("decide_duplicate answered %s, which does not keep its own schema: %s", dismissedOut, defect)
	}
	dismissed := answered[struct {
		Disposition string `json:"disposition"`
	}](t, dismissedOut)
	if dismissed.Disposition != "not_a_duplicate" {
		t.Fatalf("after not_the_same the pair is %q, want not_a_duplicate", dismissed.Disposition)
	}
	if _, err := invoke("decide_duplicate", `{"candidate_id":"`+candidate+`","decision":"not_the_same"}`); err == nil {
		t.Fatal("a second dismissal of a settled pair succeeded; it must be a conflict")
	}

	reopened := answered[struct {
		Disposition string `json:"disposition"`
	}](t, mustInvoke(t, invoke, "decide_duplicate",
		`{"candidate_id":"`+candidate+`","decision":"reopen"}`))
	if reopened.Disposition != "open" {
		t.Fatalf("after reopen the pair is %q, want open", reopened.Disposition)
	}
}

func TestACreateOffersAWordOnlyAsItExistsAndNeverAppliesIt(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "tag offerer", "read", "write"))

	word := answered[struct {
		TagID string `json:"tag_id"`
	}](t, mustInvoke(t, invoke, "create_tag", `{"name":"K5 Conference 2026"}`))

	created := answered[createdWithFollowups](t, mustInvoke(t, invoke, "create_record",
		`{"record_type":"contact","offer_tag":"k5 conference 2026","fields":{"full_name":"Ines Falk"}}`))
	if created.TagOffer == nil || !created.TagOffer.Exists || created.TagOffer.TagID == nil || *created.TagOffer.TagID != word.TagID {
		t.Fatalf("offer = %+v, want the existing tag %s, matched case-insensitively", created.TagOffer, word.TagID)
	}

	carried := answered[struct {
		Tags []struct {
			TagID string `json:"tag_id"`
		} `json:"tags"`
	}](t, mustInvoke(t, invoke, "get_record_tags",
		`{"record_type":"contact","record_id":"`+created.ID+`"}`))
	if len(carried.Tags) != 0 {
		t.Fatalf("the record carries %d tags after an OFFER; nothing may be applied until the user says yes", len(carried.Tags))
	}

	unknown := answered[createdWithFollowups](t, mustInvoke(t, invoke, "create_record",
		`{"record_type":"contact","offer_tag":"A Word Nobody Coined","fields":{"full_name":"Jonas Wirth"}}`))
	if unknown.TagOffer == nil || unknown.TagOffer.Exists || unknown.TagOffer.TagID != nil {
		t.Fatalf("offer = %+v, want a word the workspace does not have", unknown.TagOffer)
	}

	none := answered[createdWithFollowups](t, mustInvoke(t, invoke, "create_record",
		`{"record_type":"contact","fields":{"full_name":"Karla Roth"}}`))
	if none.TagOffer != nil {
		t.Fatalf("a create with no offer_tag answered an offer: %+v", none.TagOffer)
	}
}
