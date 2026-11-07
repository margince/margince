// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type recordingCases struct{ asked []ContactCaseCommand }

func (c *recordingCases) PreviewContactCaseMerge(_ context.Context, cmd ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error) {
	c.asked = append(c.asked, cmd)
	return crmcontracts.ContactCaseOutcome{SurvivorId: openapi_types.UUID(cmd.SurvivorID)}, nil
}

func (c *recordingCases) MergeContactCase(_ context.Context, cmd ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error) {
	c.asked = append(c.asked, cmd)
	return crmcontracts.ContactCaseOutcome{Committed: true, SurvivorId: openapi_types.UUID(cmd.SurvivorID)}, nil
}

func caseCallArgs(member, survivor ids.UUID, preview bool) json.RawMessage {
	args, err := json.Marshal(map[string]any{
		"record_type": "contact", "contact_id": member, "member_ids": []ids.UUID{member, survivor},
		"survivor_id": survivor, "owner_id": survivor, "revision": "r1", "preview": preview,
	})
	if err != nil {
		panic(err)
	}
	return args
}

// merge_records with member_ids runs the case engine, a preview through its
// preview and a merge through its merge, and answers the outcome.
func TestMergeRecordsDecidesAContactCaseThroughTheCaseEngine(t *testing.T) {
	member, survivor := ids.NewV7(), ids.NewV7()
	for _, preview := range []bool{true, false} {
		cases := &recordingCases{}
		out, err := mergeRecords{cases: cases}.Handle(context.Background(), caseCallArgs(member, survivor, preview))
		if err != nil {
			t.Fatalf("preview=%t: %v", preview, err)
		}
		var result MergeRecordsResult
		if err := json.Unmarshal(out, &result); err != nil {
			t.Fatal(err)
		}
		if len(cases.asked) != 1 || cases.asked[0].Preview != preview || cases.asked[0].ContactID != member {
			t.Fatalf("preview=%t: the engine was asked %+v", preview, cases.asked)
		}
		if result.Merged == preview || result.SurvivorID != survivor || result.Case == nil {
			t.Errorf("preview=%t: answered %+v", preview, result)
		}
	}
}

// A call naming both a pair and a case is refused, as is a case of one.
func TestMergeRecordsRefusesAMixedOrSingleMemberCase(t *testing.T) {
	member, survivor := ids.NewV7(), ids.NewV7()
	mixed := `{"record_type":"contact","source_id":"` + member.String() + `","member_ids":["` +
		member.String() + `","` + survivor.String() + `"],"survivor_id":"` + survivor.String() + `"}`
	single := `{"record_type":"contact","contact_id":"` + member.String() + `","member_ids":["` +
		survivor.String() + `"],"survivor_id":"` + survivor.String() + `"}`
	for name, args := range map[string]string{"mixed": mixed, "single": single} {
		cases := &recordingCases{}
		_, err := mergeRecords{cases: cases}.Handle(context.Background(), json.RawMessage(args))
		var bad *BadArgsError
		if !errors.As(err, &bad) || len(cases.asked) != 0 {
			t.Errorf("%s: answered %v and asked the engine %d time(s)", name, err, len(cases.asked))
		}
	}
}

// resolve_duplicates sends a case decision to merge_records.
func TestResolveDuplicatesRoutesACaseToMergeRecords(t *testing.T) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(caseCallArgs(ids.NewV7(), ids.NewV7(), false), &members); err != nil {
		t.Fatal(err)
	}
	if target, err := (resolveDuplicates{}).chooseTarget(members); err != nil || target != mergeTwoRecords {
		t.Errorf("a case decision routed to %q (%v), want %s", target, err, mergeTwoRecords)
	}
	members["candidate_id"] = json.RawMessage(`"` + ids.NewV7().String() + `"`)
	if _, err := (resolveDuplicates{}).chooseTarget(members); err == nil {
		t.Error("a call naming a flagged pair and a case was routed")
	}
}
