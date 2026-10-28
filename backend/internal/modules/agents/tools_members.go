// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

const memberAdminObject = "user_admin"

const (
	toolMergeMembers = "merge_members"
	memberModeMerge  = "merge"
)

// MemberManager keeps member retirement on the same engine as the web surface.
type MemberManager interface {
	Preview(context.Context, ids.UUID, *ids.UUID, bool) (crmcontracts.MemberRetirementPreview, error)
	Merge(context.Context, ids.UUID, ids.UUID, string) error
}

type memberTool struct {
	members  MemberManager
	preview  bool
	language baselanguage.Resolver
}

type memberToolArgs struct {
	SourceID     ids.UUID  `json:"source_id"`
	TargetID     *ids.UUID `json:"to_user_id,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	PreviewToken string    `json:"preview_token,omitempty"`
}

func (t memberTool) Spec() mcp.ToolSpec {
	spec := mcp.ToolSpec{
		Name: toolMergeMembers, Title: "Merge duplicate members", Version: toolVersionV1,
		Description:   "Transfer an unused imported or invited duplicate member's work to the selected account and remove the duplicate. Use the token from preview_member_retirement after the user has confirmed which account survives. The survivor keeps its sign-in settings and access; original attribution remains in history. Use resolve_duplicates for customer contacts or companies.",
		Instead:       "Use resolve_duplicates for customer contacts or companies.",
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierConfirmationRequired, WaitsEvenForItsUser: true, Unlisted: true,
		OpenAPIOp: "mergeMembers", ReplayGrant: &mcp.ReplayGrant{Object: memberAdminObject, Action: principal.ActionDelete},
		InputSchema:  schema(`{"type":"object","required":["source_id","to_user_id","preview_token"],"properties":{"source_id":{"type":"string","format":"uuid"},"to_user_id":{"type":"string","format":"uuid"},"preview_token":{"type":"string","minLength":1},"approval_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		OutputSchema: schema(`{"type":"object","required":["merged"],"properties":{"merged":{"type":"boolean"}},"additionalProperties":false}`),
	}
	if t.preview {
		spec.Name = "preview_member_retirement"
		spec.Title = "Preview removing or merging a member"
		spec.Description = "Read the work and dependencies affected by retiring a member. Requires an administrator with member read and delete permission. Set mode to merge and include to_user_id to choose the surviving member. Return the preview to the user before merge_members. Use list_colleagues to find the member accounts first."
		spec.Instead = "Use list_colleagues to find the member accounts first."
		spec.RequiredScope = principal.ScopeRead
		spec.Tier = mcp.TierAutoExecute
		spec.WaitsEvenForItsUser = false
		spec.OpenAPIOp = "previewMemberRetirement"
		spec.ReplayGrant = nil
		spec.InputSchema = schema(`{"type":"object","required":["source_id"],"properties":{"source_id":{"type":"string","format":"uuid"},"to_user_id":{"type":"string","format":"uuid"},"mode":{"type":"string","enum":["merge","remove"],"default":"remove"}},"additionalProperties":false}`)
		spec.OutputSchema = schemaFor[crmcontracts.MemberRetirementPreview]()
	}
	return spec
}

func (t memberTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	return t.runMemberCall(ctx, in)
}

func (t memberTool) runMemberCall(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args memberToolArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if args.SourceID.IsZero() {
		return nil, apperrors.ErrInvalidArgument
	}
	if t.preview {
		if args.Mode != "" && args.Mode != memberModeMerge && args.Mode != "remove" {
			return nil, apperrors.ErrInvalidArgument
		}
		out, err := t.members.Preview(ctx, args.SourceID, args.TargetID, args.Mode == memberModeMerge)
		if err != nil {
			return nil, err
		}
		return json.Marshal(out)
	}
	if err := validateMemberMergeArgs(args); err != nil {
		return nil, err
	}
	if err := t.members.Merge(ctx, args.SourceID, *args.TargetID, args.PreviewToken); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Merged bool `json:"merged"`
	}{true})
}

// RegisterMemberTools separates the read-only preview from confirmation-required merging.
func RegisterMemberTools(registry *Registry, members MemberManager) {
	registry.Register(memberPreviewTool{tool: memberTool{members: members, preview: true, language: registry.BaseLanguage()}})
	registry.Register(memberTool{members: members, language: registry.BaseLanguage()})
}

func (t memberTool) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args memberToolArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	if err := validateMemberMergeArgs(args); err != nil {
		return StageInfo{}, err
	}
	preview, err := t.members.Preview(ctx, args.SourceID, args.TargetID, true)
	if err != nil {
		return StageInfo{}, err
	}
	if preview.Token != args.PreviewToken {
		return StageInfo{}, apperrors.ErrVersionSkew
	}
	if len(preview.Blockers) > 0 || preview.SurvivorName == nil || preview.SurvivorEmail == nil {
		return StageInfo{}, apperrors.ErrConflict
	}
	return StageInfo{TargetType: memberAdminObject, TargetID: args.SourceID, Summary: fmt.Sprintf(summaryIn(ctx, t.language).mergeMembers, preview.SourceName, preview.SourceEmail, *preview.SurvivorName, *preview.SurvivorEmail)}, nil
}

type memberPreviewTool struct{ tool memberTool }

func (t memberPreviewTool) Spec() mcp.ToolSpec { return t.tool.Spec() }
func (t memberPreviewTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	return t.tool.runMemberCall(ctx, in)
}
func (t memberTool) RecordTypeOf(json.RawMessage) string { return memberAdminObject }
func (t memberTool) ServesRecordType(kind string) bool   { return kind == memberAdminObject }

func validateMemberMergeArgs(args memberToolArgs) error {
	if err := httperr.RequireBodyID("source_id", args.SourceID); err != nil {
		return err
	}
	var target ids.UUID
	if args.TargetID != nil {
		target = *args.TargetID
	}
	if err := httperr.RequireBodyID("to_user_id", target); err != nil {
		return err
	}
	if args.PreviewToken == "" {
		return httperr.Validation("preview_token", "required", "preview_token is required")
	}
	return nil
}
