// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// advance_deal, and the win-evidence arguments it shares with progress_deal.
//
// It sits beside tools_progress.go rather than in tools.go with the other §2.1
// verbs because the two deal-move doors are one subject: they stage the same
// command, share a tier resolver, and must offer the same claim about a win
// with no agreement behind it. A reader checking that they agree should not
// have to hold two files apart to do it.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- advance_deal (🟢→🟡 TierDynamic) ---

type advanceDealArgs struct {
	DealID     ids.UUID `json:"deal_id"`
	ToStageID  ids.UUID `json:"to_stage_id"`
	LostReason *string  `json:"lost_reason"`
	IfVersion  *int64   `json:"if_version"`
	WinEvidenceArgs
}

// WinEvidenceArgs is what a caller says about a win that has no agreement
// behind it, shared by every tool that moves a deal so the surface asks one
// question one way.
//
// It travels with the MOVE, not with the tool: the win gate refuses a paperless
// close on the target stage's semantic, so a door that stages a move without
// carrying this field offers a refusal its caller cannot answer.
type WinEvidenceArgs struct {
	WonWithoutContractReason *string `json:"won_without_contract_reason"`
	WonWithoutContractDetail *string `json:"won_without_contract_detail"`
}

// winEvidenceProperties is the win-evidence arguments, spelled once for the
// tools that move a deal. One question asked one way, and one place to change
// it.
//
// Deliberately terse: it is written into the system prompt of every step of
// every run carrying one of these tools, so the catalog budget
// (docs/reference/agent-tool-budget.md) is spent on it whether or not a caller
// ever sets a field. The refusal it names is enforced by the win gate, not by
// this text.
//
// Held by: TestEveryDealMoveCarriesTheWinEvidenceClaim
// (backend/gates/winevidencedoors_test.go)
const winEvidenceProperties = `,
	"won_without_contract_reason":{"type":"string","enum":["imported","purchase_order","verbal","renewal_by_email","other"],"description":"Why this win has no contract behind it. Omit when the deal has a signed contract with its paper attached; a win claiming neither is refused."},
	"won_without_contract_detail":{"type":"string","description":"What the reason was, required when it is other"}`

type advanceDeal struct {
	p      datasource.SystemOfRecordProvider
	stages StageResolver
}

func (t advanceDeal) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "advance_deal", Title: "Advance a deal to a stage", Version: toolVersionV1,
		Description:   advanceDealCopy.render(),
		RequiredScope: principal.ScopeWrite,
		Tier:          mcp.TierDynamic,
		TierResolver:  advanceDealTier,
		OpenAPIOp:     "advanceDeal",
		InputSchema: schema(`{"type":"object","required":["deal_id","to_stage_id"],"properties":{
			"deal_id":{"type":"string","format":"uuid"},
			"to_stage_id":{"type":"string","format":"uuid"` + stageIDNote + `},
			"lost_reason":{"type":"string","description":"Required when the target stage closes the deal as lost"}` + winEvidenceProperties + `,
			"if_version":{"type":"integer"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on retry after a human approved a won/lost move"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[wireRecord](),
	}
}

// ResolverInput reads the target stage's semantic from pipeline config —
// a renamed "Won" column still resolves 🟡, because the semantic, not the
// label or the request, is what the gate trusts.
func (t advanceDeal) ResolverInput(ctx context.Context, in json.RawMessage) (mcp.TierResolverInput, error) {
	var args advanceDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return mcp.TierResolverInput{}, err
	}
	return DealMoveTierInput(ctx, t.p, t.stages, args.DealID, args.ToStageID, in)
}

// StageInfo decodes this door's arguments into the deal-move command and
// delegates: the refusals and the staged subject — including the version pin,
// so an approval given for "close this deal as it stands" cannot execute
// against a deal that changed in between — live in the resolver
// (commandlifecycle.go), where the REST door reaches the same ones for the
// same operation.
func (t advanceDeal) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args advanceDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewAdvanceDealCall(t.p, t.stages, AdvanceDealCommand{
		DealID:    args.DealID,
		ToStageID: args.ToStageID,
	}))
}

func (t advanceDeal) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args advanceDealArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	pin, err := pinForWrite(ctx, args.IfVersion)
	if err != nil {
		return nil, err
	}
	ref, err := t.p.AdvanceDeal(ctx, datasource.AdvanceDealInput{
		WonWithoutContractReason: args.WonWithoutContractReason,
		WonWithoutContractDetail: args.WonWithoutContractDetail,
		DealID:                   args.DealID,
		ToStageID:                args.ToStageID,
		LostReason:               args.LostReason,
		Source:                   ToolSource,
		IfVersion:                pin,
	})
	if err != nil {
		return nil, err
	}
	return readBack(ctx, t.p, ref)
}

// readBack answers every write with the resulting record — the agent
// needs the post-write state (server-derived fields, bumped version)
// without a second round-trip.
func readBack(ctx context.Context, p datasource.SystemOfRecordProvider, ref datasource.EntityRef) (json.RawMessage, error) {
	return marshalResult(readBackRecord(ctx, p, ref))
}

// readBackRecord is the same read, answered as the record rather than as its
// bytes — for the callers that carry it INSIDE a larger result and would
// otherwise have to splice one encoded document into another.
func readBackRecord(ctx context.Context, p datasource.SystemOfRecordProvider, ref datasource.EntityRef) (wireRecord, error) {
	rec, err := p.Read(ctx, ref)
	if err != nil {
		return wireRecord{}, fmt.Errorf("crmagents: write landed but read-back failed: %w", err)
	}
	return newWireRecord(ctx, rec), nil
}
