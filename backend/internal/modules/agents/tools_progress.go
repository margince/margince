// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// progress_deal (interfaces.md §2.2): the intent-level composition of
// advance_deal + log_activity — one verb for "move the deal and note
// why". It inherits advance_deal's TierDynamic resolver unchanged (🟢
// open→open, 🟡 to won/lost), because an intent composition never widens
// authority beyond the §2.1 calls it composes.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

type progressDealArgs struct {
	DealID     ids.UUID `json:"deal_id"`
	ToStageID  ids.UUID `json:"to_stage_id"`
	LostReason *string  `json:"lost_reason"`
	Note       *string  `json:"note"`
	IfVersion  *int64   `json:"if_version"`
	// Settle answers the question a close owes: what becomes of the deal's
	// open tasks. "complete" ticks them done, "keep" leaves them. Absent is
	// "keep", so a caller that never heard the question settles nothing.
	Settle *string `json:"settle"`
	WinEvidenceArgs
}

type progressDeal struct {
	p          datasource.SystemOfRecordProvider
	stages     StageResolver
	language   baselanguage.Resolver
	closeTasks DealTaskSettler
}

func (t progressDeal) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "progress_deal", Title: "Progress a deal with a note", Version: toolVersionV1,
		Description:   progressDealCopy.render(),
		Instead:       progressDealCopy.Instead,
		RequiredScope: principal.ScopeWrite,
		Unlisted:      true,
		Tier:          mcp.TierDynamic,
		// The SAME resolver as advance_deal: the intent composition never
		// widens authority, so the won/lost 🟡 floor holds identically.
		TierResolver: advanceDealTier,
		OpenAPIOp:    "progressDeal",
		InputSchema: schema(`{"type":"object","required":["deal_id","to_stage_id"],"properties":{
			"deal_id":{"type":"string","format":"uuid"},
			"to_stage_id":{"type":"string","format":"uuid"` + stageIDNote + `},
			"lost_reason":{"type":"string","description":"Required when the target stage closes the deal as lost"}` + winEvidenceProperties + `,
			"note":{"type":"string","description":"Logged as a note on the deal's timeline after the move"},
			"if_version":{"type":"integer"},
			"settle":{"type":"string","enum":["complete","keep"],"description":"What becomes of the deal's open tasks when this move closes it. Omitted keeps them"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on retry after a human approved a won/lost move"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ProgressDealResult](),
	}
}

// ResolverInput mirrors advance_deal's: the target stage's configured
// semantic decides the tier, never the request's labels.
func (t progressDeal) ResolverInput(ctx context.Context, in json.RawMessage) (mcp.TierResolverInput, error) {
	var args progressDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return mcp.TierResolverInput{}, err
	}
	// The SAME builder advance_deal uses: this tool shares that tool's resolver,
	// so it has to feed it the same two endpoints, and a second copy is how the
	// shared rule comes to hold on one tool and not the other.
	return DealMoveTierInput(ctx, t.p, t.stages, args.DealID, args.ToStageID, in)
}

// StageInfo decodes this door's arguments into the SAME command advance_deal
// stages (AdvanceDealCommand, commandlifecycle.go) and delegates.
//
// The same command, not a same-shaped one: the two tools stage the same act,
// so a human reading an inbox should not have to know which tool proposed a
// move to understand what approving it does — and the note this intent adds
// afterwards is not part of the move, so it changes neither the target, the
// pin, nor the sentence.
func (t progressDeal) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args progressDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewAdvanceDealCall(t.p, t.stages, t.language, AdvanceDealCommand{
		DealID:    args.DealID,
		ToStageID: args.ToStageID,
	}))
}

func (t progressDeal) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args progressDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	pin, err := pinForWrite(ctx, args.IfVersion)
	if err != nil {
		return nil, err
	}
	if _, err := t.p.AdvanceDeal(ctx, datasource.AdvanceDealInput{
		DealID:                   args.DealID,
		ToStageID:                args.ToStageID,
		LostReason:               args.LostReason,
		WonWithoutContractReason: args.WonWithoutContractReason,
		WonWithoutContractDetail: args.WonWithoutContractDetail,
		Source:                   ToolSource,
		IfVersion:                pin,
	}); err != nil {
		return nil, err
	}
	var result ProgressDealResult
	if args.Note != nil && strings.TrimSpace(*args.Note) != "" {
		fields, err := json.Marshal(map[string]any{
			"kind": "note",
			"body": strings.TrimSpace(*args.Note),
			"links": []map[string]any{
				{"entity_type": "deal", "entity_id": args.DealID},
			},
		})
		if err != nil {
			return nil, err
		}
		ref, err := t.p.Create(ctx, datasource.CreateInput{
			EntityType: datasource.EntityActivity,
			Fields:     fields,
			Source:     ToolSource,
		})
		if err != nil {
			return nil, fmt.Errorf("crmagents: deal advanced but logging the note failed — the move stands, retry via log_activity: %w", err)
		}
		result.NoteActivityID = &ref.ID
		noteEvidence(ctx, datasource.EntityActivity, ref.ID)
	}
	if err := t.answerForOpenTasks(ctx, args, &result); err != nil {
		return nil, err
	}
	deal, err := readBackRecord(ctx, t.p, datasource.EntityRef{Type: datasource.EntityDeal, ID: args.DealID})
	if err != nil {
		return nil, err
	}
	result.Deal = deal
	return json.Marshal(result)
}

// settleComplete is the choice that ticks a closing deal's open tasks done.
const settleComplete = "complete"

// answerForOpenTasks fills the close's answer about the deal's open tasks.
//
// Only a move that CLOSES asks. An ordinary stage change leaves the work where
// it was, and dealmove reads a non-open target as the close.
//
// Reported rather than settled unless the caller said so, because closing a
// deal never settles its tasks silently.
func (t progressDeal) answerForOpenTasks(
	ctx context.Context, args progressDealArgs, result *ProgressDealResult,
) error {
	if t.closeTasks == nil {
		return nil
	}
	semantic, _, err := t.stages.StageSemantic(ctx, args.ToStageID)
	if err != nil {
		return err
	}
	if semantic == stageSemanticOpen {
		return nil
	}
	if args.Settle != nil && *args.Settle == settleComplete {
		settled, err := t.closeTasks.CompleteOpenTasksOnDeal(ctx, args.DealID)
		if err != nil {
			return err
		}
		result.SettledTaskIDs = settled
		return nil
	}
	open, err := t.closeTasks.OpenTasksOnDeal(ctx, args.DealID)
	if err != nil {
		return err
	}
	result.OpenTasks = open
	return nil
}
