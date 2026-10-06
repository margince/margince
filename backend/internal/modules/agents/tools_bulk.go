// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// bulk_update_records: one change over a selection of contacts, companies,
// deals, leads or Worklist tasks, previewed and confirmed in the conversation that asked for it.
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

// BulkChanger is the bulk-change engine. Every method answers the contract's
// own shape, already encoded.
type BulkChanger interface {
	PreviewBulkChange(ctx context.Context, cmd BulkChangeCommand) (json.RawMessage, error)
	ExecuteBulkChange(ctx context.Context, cmd BulkChangeCommand) (json.RawMessage, error)
	PreviewBulkUndo(ctx context.Context, batchID ids.UUID) (json.RawMessage, error)
	UndoBulkChange(ctx context.Context, batchID ids.UUID, confirmToken string) (json.RawMessage, error)
}

// BulkChangeCommand is one bulk change, whichever door asked for it.
type BulkChangeCommand struct {
	RecordType   string
	Verb         string
	Items        []BulkItem
	OwnerID      *ids.UUID
	ListID       *ids.UUID
	Note         *string
	TagID        *ids.UUID
	Task         *BulkTask
	ConfirmToken string
}

// BulkTask is the task create_task files under every record.
type BulkTask struct {
	Subject    string     `json:"subject"`
	DueAt      *time.Time `json:"due_at,omitempty"`
	AssigneeID *ids.UUID  `json:"assignee_id,omitempty"`
}

// BulkItem is one selected record and the version the caller read.
type BulkItem struct {
	ID      ids.UUID `json:"id"`
	Version int64    `json:"version"`
}

// The modes of the tool: a change previewed and executed, and its undo
// previewed and executed.
const (
	bulkModePreview     = "preview"
	bulkModeExecute     = "execute"
	bulkModeUndoPreview = "undo_preview"
	bulkModeUndo        = "undo"
)

// BulkSkip is one record a bulk change leaves alone, and why.
type BulkSkip struct {
	ID      ids.UUID `json:"id"`
	Reason  string   `json:"reason"`
	Message string   `json:"message,omitempty"`
}

// BulkRecordState is the facts a bulk change can move on a record.
type BulkRecordState struct {
	OwnerID  *ids.UUID `json:"owner_id"`
	Archived bool      `json:"archived"`
	Listed   *bool     `json:"listed,omitempty"`
	Tagged   *bool     `json:"tagged,omitempty"`
	TaskID   *ids.UUID `json:"task_id,omitempty"`
	Done     *bool     `json:"done,omitempty"`
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
	RecordType           string           `json:"record_type,omitempty"`
	Verb                 string           `json:"verb,omitempty"`
	Count                *int             `json:"count,omitempty"`
	Affected             []ids.UUID       `json:"affected,omitempty"`
	Excluded             []BulkSkip       `json:"excluded,omitempty"`
	Sample               []BulkSampleRow  `json:"sample,omitempty"`
	RequiresConfirmation *bool            `json:"requires_confirmation,omitempty"`
	ConfirmToken         string           `json:"confirm_token,omitempty"`
	ExpiresAt            *time.Time       `json:"expires_at,omitempty"`
	BatchID              *ids.UUID        `json:"batch_id,omitempty"`
	Changed              *int             `json:"changed,omitempty"`
	Skipped              []BulkSkip       `json:"skipped,omitempty"`
	LeftBehind           []BulkLeftBehind `json:"left_behind,omitempty"`
	UndoOf               *ids.UUID        `json:"undo_of,omitempty"`
}

// BulkLeftBehind is one thing an undo restored a record without.
type BulkLeftBehind struct {
	ID    ids.UUID `json:"id"`
	Kind  string   `json:"kind"`
	RefID ids.UUID `json:"ref_id"`
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
	ListID       *ids.UUID  `json:"list_id"`
	Note         *string    `json:"note"`
	TagID        *ids.UUID  `json:"tag_id"`
	Task         *BulkTask  `json:"task"`
	ConfirmToken string     `json:"confirm_token"`
	BatchID      *ids.UUID  `json:"batch_id"`
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
		InputSchema: schema(`{"type":"object","required":["mode"],"properties":{
			"mode":{"type":"string","enum":["preview","execute","undo_preview","undo"],
				"description":"preview says what would change; execute changes it; undo_preview and undo do the same for putting back the change batch_id names"},
			"record_type":{"type":"string","enum":["contact","company","deal","lead","worklist_item"],"description":"A lead takes every verb but archive. A worklist_item is a Worklist task, and takes complete alone; a Worklist commitment is refused, because the user marks it done"},
			"verb":{"type":"string","enum":["reassign_owner","archive","add_to_list","remove_from_list","add_tag","remove_tag","create_task","complete"]},
			"items":{"type":"array","minItems":1,"maxItems":500,"items":{"type":"object","required":["id","version"],
				"properties":{"id":{"type":"string","format":"uuid"},"version":{"type":"integer"}},"additionalProperties":false}},
			"owner_id":{"type":"string","format":"uuid","description":"The new owner, for reassign_owner"},
			"list_id":{"type":"string","format":"uuid","description":"The Shortlist, for add_to_list and remove_from_list"},
			"note":{"type":"string","maxLength":500,"description":"Why, for add_to_list and remove_from_list"},
			"tag_id":{"type":"string","format":"uuid","description":"The tag, for add_tag and remove_tag"},
			"task":{"type":"object","required":["subject"],"description":"The task create_task files under each record",
				"properties":{"subject":{"type":"string","minLength":1,"maxLength":500},
					"due_at":{"type":"string","format":"date-time"` + timestampNote + `},
					"assignee_id":{"type":"string","format":"uuid","description":"Who owes it; defaults to the user"}},
				"additionalProperties":false},
			"confirm_token":{"type":"string","description":"The token preview or undo_preview answered; needed above 10 records"},
			"batch_id":{"type":"string","format":"uuid","description":"For undo_preview and undo: the batch_id execute answered"}},
			"if":{"properties":{"mode":{"enum":["preview","execute"]}}},
			"then":{"required":["record_type","verb","items"]},
			"else":{"required":["batch_id"]},
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
		OwnerID: args.OwnerID, ListID: args.ListID, Note: args.Note, TagID: args.TagID, Task: args.Task,
		ConfirmToken: args.ConfirmToken,
	}
	switch args.Mode {
	case bulkModePreview, bulkModeExecute:
		if args.BatchID != nil {
			return nil, &BadArgsError{Cause: fmt.Errorf("batch_id belongs to undo_preview and undo")}
		}
		if args.RecordType == "" || args.Verb == "" || len(args.Items) == 0 {
			return nil, &BadArgsError{Cause: fmt.Errorf("%s needs record_type, verb and items", args.Mode)}
		}
		if args.Mode == bulkModeExecute {
			return t.changer.ExecuteBulkChange(ctx, cmd)
		}
		if cmd.ConfirmToken != "" {
			return nil, &BadArgsError{Cause: fmt.Errorf("confirm_token belongs to execute; a preview mints one")}
		}
		return t.changer.PreviewBulkChange(ctx, cmd)
	case bulkModeUndoPreview, bulkModeUndo:
		return t.undo(ctx, args)
	}
	return nil, &BadArgsError{Cause: fmt.Errorf("mode %q is none of %q, %q, %q or %q",
		args.Mode, bulkModePreview, bulkModeExecute, bulkModeUndoPreview, bulkModeUndo)}
}

// undo previews or runs the undo of one batch, which names nothing but the
// batch: the records, the verb and the owners are the batch's own.
func (t bulkUpdateRecords) undo(ctx context.Context, args bulkUpdateRecordsArgs) (json.RawMessage, error) {
	if args.BatchID == nil {
		return nil, &BadArgsError{Cause: fmt.Errorf("%s needs batch_id, the batch execute answered", args.Mode)}
	}
	if args.RecordType != "" || args.Verb != "" || len(args.Items) > 0 || args.OwnerID != nil || args.ListID != nil ||
		args.TagID != nil || args.Task != nil {
		return nil, &BadArgsError{Cause: fmt.Errorf("%s takes only batch_id and confirm_token; the batch names the rest", args.Mode)}
	}
	if args.Mode == bulkModeUndo {
		return t.changer.UndoBulkChange(ctx, *args.BatchID, args.ConfirmToken)
	}
	if args.ConfirmToken != "" {
		return nil, &BadArgsError{Cause: fmt.Errorf("confirm_token belongs to undo; undo_preview mints one")}
	}
	return t.changer.PreviewBulkUndo(ctx, *args.BatchID)
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
