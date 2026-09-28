// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// bulk_update_records: one change over a selection of contacts, companies or
// deals, previewed and confirmed in the conversation that asked for it.
//
// The tool reaches the engine POST /v1/bulk/preview and POST /v1/bulk/execute
// run on, through a seam compose implements, so the per-row write check, the
// single-record rules and the confirmation token are the engine's own and never
// restated here.
//
// The confirmation is the user's, given in the conversation: preview answers
// a confirm_token for a selection of more than ten records, the agent shows the
// user what the change will do, and executes with the token only after they
// agree. That is why the tool never stages into the approval inbox, and why an
// exhausted write budget refuses it outright (ConfirmsInConversation).

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// BulkChanger is the bulk-change engine. Both methods answer the contract's
// own shape, already encoded.
type BulkChanger interface {
	PreviewBulkChange(ctx context.Context, cmd BulkChangeCommand) (json.RawMessage, error)
	ExecuteBulkChange(ctx context.Context, cmd BulkChangeCommand) (json.RawMessage, error)
}

// BulkChangeCommand is one bulk change, whichever door asked for it.
type BulkChangeCommand struct {
	RecordType   string
	Verb         string
	Items        []BulkItem
	OwnerID      *ids.UUID
	ConfirmToken string
}

// BulkItem is one selected record and the version the caller read.
type BulkItem struct {
	ID      ids.UUID `json:"id"`
	Version int64    `json:"version"`
}

// The two modes of the tool.
const (
	bulkModePreview = "preview"
	bulkModeExecute = "execute"
)

// BulkSkip is one record a bulk change leaves alone, and why.
type BulkSkip struct {
	ID      ids.UUID `json:"id"`
	Reason  string   `json:"reason"`
	Message string   `json:"message,omitempty"`
}

// BulkRecordState is the two facts a bulk change can move on a record.
type BulkRecordState struct {
	OwnerID  *ids.UUID `json:"owner_id"`
	Archived bool      `json:"archived"`
}

// BulkSampleRow is one record the change would alter, before and after.
type BulkSampleRow struct {
	ID     ids.UUID        `json:"id"`
	Label  string          `json:"label"`
	Before BulkRecordState `json:"before"`
	After  BulkRecordState `json:"after"`
}

// BulkChangeAnswer is what the tool answers: a preview's members in preview
// mode, an execution's in execute mode. It is one type because a tool
// advertises one output shape.
type BulkChangeAnswer struct {
	RecordType           string          `json:"record_type,omitempty"`
	Verb                 string          `json:"verb,omitempty"`
	Count                *int            `json:"count,omitempty"`
	Affected             []ids.UUID      `json:"affected,omitempty"`
	Excluded             []BulkSkip      `json:"excluded,omitempty"`
	Sample               []BulkSampleRow `json:"sample,omitempty"`
	RequiresConfirmation *bool           `json:"requires_confirmation,omitempty"`
	ConfirmToken         string          `json:"confirm_token,omitempty"`
	ExpiresAt            *time.Time      `json:"expires_at,omitempty"`
	BatchID              *ids.UUID       `json:"batch_id,omitempty"`
	Changed              *int            `json:"changed,omitempty"`
	Skipped              []BulkSkip      `json:"skipped,omitempty"`
}

// RegisterBulkTool wires bulk_update_records over the engine's seam.
func RegisterBulkTool(r *Registry, changer BulkChanger) {
	r.Register(bulkUpdateRecords{changer: changer})
}

type bulkUpdateRecordsArgs struct {
	Mode         string     `json:"mode"`
	RecordType   string     `json:"record_type"`
	Verb         string     `json:"verb"`
	Items        []BulkItem `json:"items"`
	OwnerID      *ids.UUID  `json:"owner_id"`
	ConfirmToken string     `json:"confirm_token"`
}

type bulkUpdateRecords struct{ changer BulkChanger }

func (t bulkUpdateRecords) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "bulk_update_records", Title: "Change many records at once", Version: toolVersionV1,
		Description:   bulkUpdateRecordsCopy.render(),
		Instead:       bulkUpdateRecordsCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp:              "executeBulkChange",
		ConfirmsInConversation: true,
		InputSchema: schema(`{"type":"object","required":["mode","record_type","verb","items"],"properties":{
			"mode":{"type":"string","enum":["preview","execute"],"description":"preview says what would change; execute changes it"},
			"record_type":{"type":"string","enum":["contact","company","deal"]},
			"verb":{"type":"string","enum":["reassign_owner","archive"]},
			"items":{"type":"array","minItems":1,"maxItems":500,"items":{"type":"object","required":["id","version"],
				"properties":{"id":{"type":"string","format":"uuid"},"version":{"type":"integer"}},"additionalProperties":false}},
			"owner_id":{"type":"string","format":"uuid","description":"The new owner, for reassign_owner"},
			"confirm_token":{"type":"string","description":"The token preview answered; needed above 10 records"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[BulkChangeAnswer](),
	}
}

func (t bulkUpdateRecords) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args bulkUpdateRecordsArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	cmd := BulkChangeCommand{
		RecordType: args.RecordType, Verb: args.Verb, Items: args.Items,
		OwnerID: args.OwnerID, ConfirmToken: args.ConfirmToken,
	}
	switch args.Mode {
	case bulkModePreview:
		if cmd.ConfirmToken != "" {
			return nil, &BadArgsError{Cause: fmt.Errorf("confirm_token belongs to execute; a preview mints one")}
		}
		return t.changer.PreviewBulkChange(ctx, cmd)
	case bulkModeExecute:
		return t.changer.ExecuteBulkChange(ctx, cmd)
	}
	return nil, &BadArgsError{Cause: fmt.Errorf("mode %q is neither %q nor %q", args.Mode, bulkModePreview, bulkModeExecute)}
}

// NewBulkChangeCall binds one bulk change to the resolver that answers for it
// on the REST door.
//
//nolint:ireturn // the call IS the product: a resolver named concretely here is exactly the thing that must not leave this package
func NewBulkChangeCall(language baselanguage.Resolver, cmd BulkChangeCommand) GovernedCall {
	return bind[BulkChangeCommand](bulkChangeResolver{language: language}, cmd)
}

type bulkChangeResolver struct {
	language baselanguage.Resolver
}

// Subject names no record: a selection has no one row an approval could bind
// to. It is reached only after Guards, which refuses every staging.
func (r bulkChangeResolver) Subject(ctx context.Context, cmd BulkChangeCommand) (StageInfo, error) {
	said := summaryIn(ctx, r.language)
	return StageInfo{Summary: fmt.Sprintf(said.bulkChange, len(cmd.Items))}, nil
}

// Guards refuses to stage at all. A bulk change is confirmed in the
// conversation that asked for it, with a preview and its token, and never in
// the approval inbox; an installation that requires a human's approval for it
// has therefore closed it to agents.
func (bulkChangeResolver) Guards(context.Context, BulkChangeCommand) error {
	return fmt.Errorf("a bulk change is confirmed in the conversation with its preview's confirm_token, "+
		"never in the approval inbox, and this installation requires approval for it: %w", apperrors.ErrRequiresApproval)
}
