// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

type seamProbeMembers struct{}

func (seamProbeMembers) Preview(context.Context, ids.UUID, *ids.UUID, bool) (crmcontracts.MemberRetirementPreview, error) {
	return crmcontracts.MemberRetirementPreview{}, errSeamReached
}

func (seamProbeMembers) Merge(context.Context, ids.UUID, ids.UUID, string) error {
	return errSeamReached
}

func TestEveryRequiredMemberMergeBodyIDIsNamedWhenAbsent(t *testing.T) {
	raw, err := json.Marshal(memberToolArgs{SourceID: ids.NewV7(), PreviewToken: "reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = (memberTool{}).Handle(context.Background(), raw)
	if err == nil || !strings.Contains(err.Error(), "to_user_id") {
		t.Fatalf("missing target: %v", err)
	}
}

func TestMemberMergeRequiresConfirmationAndItsPreviewCannotStage(t *testing.T) {
	registry := NewRegistry(nil, nil)
	RegisterMemberTools(registry, seamProbeMembers{})
	if registry.Stageable("preview_member_retirement") {
		t.Fatal("a read-only preview may not stage")
	}
	if !registry.Stageable("merge_members") {
		t.Fatal("member merge has no confirmation seam")
	}
	for _, spec := range registry.Specs() {
		if spec.Name == "merge_members" && spec.Tier != mcp.TierConfirmationRequired {
			t.Fatalf("tier = %v", spec.Tier)
		}
	}
}
