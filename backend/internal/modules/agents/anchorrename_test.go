// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func anchorCandidate(id ids.UUID, fields string) datasource.Record {
	return datasource.Record{
		Ref:       datasource.EntityRef{Type: datasource.EntityCompany, ID: id},
		Fields:    json.RawMessage(fields),
		Version:   3,
		Freshness: datasource.FreshnessInfo{Authoritative: true},
	}
}

func TestPatchGuardsRefuseAMachineRenamingTheAnchorBeforeStaging(t *testing.T) {
	agent := principal.WithActor(context.Background(), principal.Principal{ID: "agent:renamer"})
	human := principal.WithActor(context.Background(), principal.Principal{ID: "human:" + ids.NewV7().String()})
	anchor := `{"is_anchor":true,"display_name":"Profile Name AG"}`
	for name, tc := range map[string]struct {
		ctx     context.Context
		record  string
		patch   string
		refused bool
	}{
		"an agent renaming the anchor":       {agent, anchor, `{"display_name":"Agent Picked Ltd"}`, true},
		"an agent resending the anchor name": {agent, anchor, `{"display_name":" Profile Name AG "}`, false},
		"an agent editing another field":     {agent, anchor, `{"industry":"payments"}`, false},
		"an agent renaming a customer":       {agent, `{"is_anchor":false,"display_name":"Brandt"}`, `{"display_name":"Brandt KG"}`, false},
		"a human renaming the anchor":        {human, anchor, `{"display_name":"Human Picked AG"}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			id := ids.NewV7()
			call := NewPatchCall(stubRecordProvider{rec: anchorCandidate(id, tc.record)}, nil, PatchCommand{
				RecordType: "company", ID: id, Fields: json.RawMessage(tc.patch),
			})

			err := call.Guards(tc.ctx)

			bad, isBad := errors.AsType[*BadArgsError](err)
			if !tc.refused {
				if err != nil {
					t.Fatalf("Guards = %v, want the patch admitted to staging", err)
				}
				return
			}
			if !isBad || bad.Field != "display_name" || bad.Code != "anchor_protected" {
				t.Fatalf("Guards = %v, want an anchor_protected refusal on display_name", err)
			}
		})
	}
}

func TestUpdateRecordRefusesAMachineRenamingTheAnchorBeforeStaging(t *testing.T) {
	agent := principal.WithActor(context.Background(), principal.Principal{ID: "agent:renamer"})
	human := principal.WithActor(context.Background(), principal.Principal{ID: "human:" + ids.NewV7().String()})
	anchor := `{"is_anchor":true,"display_name":"Profile Name AG"}`
	for name, tc := range map[string]struct {
		ctx       context.Context
		record    string
		patch     string
		humanOwns string
		refused   bool
	}{
		"an agent renaming the anchor":           {agent, anchor, `{"display_name":"Agent Picked Ltd"}`, "display_name", true},
		"an agent renaming it beside its fields": {agent, anchor, `{"display_name":"Agent Picked Ltd","industry":"payments"}`, "display_name", true},
		"an agent resending the anchor name":     {agent, anchor, `{"display_name":"Profile Name AG"}`, "display_name", false},
		"an agent overwriting another field":     {agent, anchor, `{"industry":"payments"}`, "industry", false},
		"an agent renaming a customer":           {agent, `{"is_anchor":false,"display_name":"Brandt"}`, `{"display_name":"Brandt KG"}`, "display_name", false},
		"a human renaming the anchor":            {human, anchor, `{"display_name":"Human Picked AG"}`, "display_name", false},
	} {
		t.Run(name, func(t *testing.T) {
			id := ids.NewV7()
			p := &fakeSoR{records: map[datasource.EntityRef]datasource.Record{
				{Type: datasource.EntityCompany, ID: id}: anchorCandidate(id, tc.record),
			}}
			approvals := &recordingApprovals{}
			tool := updateRecord{p: p, ownership: fixedOwnership{conflicts: []string{tc.humanOwns}}, staging: approvals}
			in, err := json.Marshal(map[string]any{"record_type": "company", "id": id, "fields": json.RawMessage(tc.patch)})
			if err != nil {
				t.Fatalf("marshaling args: %v", err)
			}

			_, err = tool.Handle(tc.ctx, in)

			if !tc.refused {
				if len(approvals.staged) != 1 {
					t.Fatalf("Handle = %v with %d approvals, want the human-owned field staged", err, len(approvals.staged))
				}
				return
			}
			if bad, isBad := errors.AsType[*BadArgsError](err); !isBad || bad.Field != "display_name" || bad.Code != "anchor_protected" {
				t.Fatalf("Handle = %v, want an anchor_protected refusal on display_name", err)
			}
			if len(approvals.staged) != 0 || len(p.updates) != 0 {
				t.Errorf("the refused rename staged %d and applied %d, want neither", len(approvals.staged), len(p.updates))
			}
		})
	}
}
