// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALinkedInMatchEditMayFlipOnlyItsTwoBoxes(t *testing.T) {
	staged := json.RawMessage(`{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
		`"contact_title":"","use_as_title":true,"use_as_employer":false}`)
	cases := []struct {
		name   string
		edited string
		ok     bool
	}{
		{"both boxes flipped", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
			`"contact_title":"","use_as_title":false,"use_as_employer":true}`, true},
		{"unchanged", string(staged), true},
		{"position rewritten", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CEO",` +
			`"contact_title":"","use_as_title":true,"use_as_employer":false}`, false},
		{"staged title rewritten", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
			`"contact_title":"Founder","use_as_title":true,"use_as_employer":false}`, false},
		{"member added", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
			`"contact_title":"","use_as_title":true,"use_as_employer":false,"contact_name":"X"}`, false},
		{"box dropped", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
			`"contact_title":"","use_as_title":true}`, false},
		{"box not a boolean", `{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_position":"CTO",` +
			`"contact_title":"","use_as_title":"yes","use_as_employer":false}`, false},
	}
	for _, tc := range cases {
		err := assertEditableMembers(kindLinkedInMatch, staged, json.RawMessage(tc.edited))
		var invalid *InvalidEditError
		if tc.ok && err != nil {
			t.Errorf("%s: refused with %v, want it admitted", tc.name, err)
		}
		if !tc.ok && !errors.As(err, &invalid) {
			t.Errorf("%s: answered %v, want an invalid-edit refusal", tc.name, err)
		}
	}
}

func TestACardStagedWithoutTheBoxesTakesNoBox(t *testing.T) {
	legacy := json.RawMessage(`{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_name":"Jane"}`)
	edited := json.RawMessage(`{"connection_id":"0190a1b2-0000-7000-8000-000000000001","connection_name":"Jane","use_as_title":true}`)
	var invalid *InvalidEditError
	if err := assertEditableMembers(kindLinkedInMatch, legacy, edited); !errors.As(err, &invalid) {
		t.Errorf("adding a box to a card staged without one answered %v, want an invalid-edit refusal", err)
	}
}

func TestAKindWithNoListKeepsTheGeneralEditRule(t *testing.T) {
	if err := assertEditableMembers("send_email", json.RawMessage(`{"subject":"a"}`), json.RawMessage(`{"subject":"b"}`)); err != nil {
		t.Errorf("a content edit on an unlisted kind was refused: %v", err)
	}
}

func TestAHandoverTakesNoEditToAnyMember(t *testing.T) {
	staged := map[string]string{
		"record_type": "company", "record_id": "0190a1b2-0000-7000-8000-000000000001",
		"from_id": "0190a1b2-0000-7000-8000-000000000002", "to_id": "0190a1b2-0000-7000-8000-000000000003",
		"requested_by": "0190a1b2-0000-7000-8000-000000000003", "decider_id": "0190a1b2-0000-7000-8000-000000000002",
		"note": "I run this account now",
	}
	raw, err := json.Marshal(staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertEditableMembers(approvalsubject.KindOwnershipTransfer, raw, raw); err != nil {
		t.Errorf("accepting the request as staged was refused: %v", err)
	}
	for member := range staged {
		edited := maps.Clone(staged)
		edited[member] = "contact"
		body, err := json.Marshal(edited)
		if err != nil {
			t.Fatal(err)
		}
		var invalid *InvalidEditError
		if err := assertEditableMembers(approvalsubject.KindOwnershipTransfer, raw, body); !errors.As(err, &invalid) {
			t.Errorf("an edit to %s answered %v, want an invalid-edit refusal", member, err)
		}
	}
}

func TestAShareRequestTakesNoEditAndNoAddedMember(t *testing.T) {
	staged := json.RawMessage(`{"contact_id":"0190a1b2-0000-7000-8000-000000000001","owner_id":"0190a1b2-0000-7000-8000-000000000002","requester_id":"0190a1b2-0000-7000-8000-000000000003"}`)
	if err := assertEditableMembers(approvalsubject.KindContactShare, staged, staged); err != nil {
		t.Errorf("accepting the request as staged was refused: %v", err)
	}
	added := json.RawMessage(`{"contact_id":"0190a1b2-0000-7000-8000-000000000001","owner_id":"0190a1b2-0000-7000-8000-000000000002","requester_id":"0190a1b2-0000-7000-8000-000000000003","note":"please"}`)
	var invalid *InvalidEditError
	if err := assertEditableMembers(approvalsubject.KindContactShare, staged, added); !errors.As(err, &invalid) {
		t.Errorf("an added member answered %v, want an invalid-edit refusal", err)
	}
}

func TestAHeldDraftEditMayNotBlankItsSubjectOrBody(t *testing.T) {
	cases := []struct {
		name   string
		kind   string
		edited string
		ok     bool
	}{
		{"both written", kindHeldDraft, `{"subject":"Hi","body":"Text"}`, true},
		{"subject blank", kindHeldDraft, `{"subject":"  ","body":"Text"}`, false},
		{"body empty", kindHeldDraft, `{"subject":"Hi","body":""}`, false},
		{"other kind keeps its own rule", kindLinkedInMatch, `{"subject":""}`, true},
	}
	for _, tc := range cases {
		err := assertMailHasText(tc.kind, json.RawMessage(tc.edited))
		var invalid *InvalidEditError
		if tc.ok && err != nil {
			t.Errorf("%s: refused: %v", tc.name, err)
		}
		if !tc.ok && !errors.As(err, &invalid) {
			t.Errorf("%s: got %v, want an InvalidEditError", tc.name, err)
		}
	}
}

func TestOnlyAnApprovedUnredeemedAgentStagingAwaitsTheAgent(t *testing.T) {
	passport := ids.New[ids.PassportKind]()
	cases := []struct {
		name string
		a    row
		want bool
	}{
		{"agent staging, approved", row{Status: approvalStatusApproved, PassportID: &passport}, true},
		{"already redeemed", row{Status: approvalStatusApproved, PassportID: &passport, ConsumedAt: new(time.Time)}, false},
		{"window lapsed", row{Status: approvalStatusApproved, PassportID: &passport, EffectFailedAt: new(time.Time)}, false},
		{"server proposed ran its effect", row{Status: approvalStatusApproved}, false},
		{"still pending", row{Status: statusPending, PassportID: &passport}, false},
	}
	for _, tc := range cases {
		if got := awaitsAgent(tc.a); got != tc.want {
			t.Errorf("%s: awaitsAgent = %v, want %v", tc.name, got, tc.want)
		}
	}
}
