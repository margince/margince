// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The update_record tool: 🟢 by tier. Every field applies in the one call, with
// the audit row and Undo as the safety net. A contract floor that tightens an
// operation to confirm-first stages the whole call through StageInfo instead.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- update_record (🟢 write) ---

type updateRecord struct {
	p        datasource.SystemOfRecordProvider
	language baselanguage.Resolver
	words    ChangeWords
}

type updateRecordArgs struct {
	RecordType string          `json:"record_type"`
	ID         ids.UUID        `json:"id"`
	Fields     json.RawMessage `json:"fields"`
	IfVersion  *int64          `json:"if_version"`
}

func (t updateRecord) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "update_record", Title: "Update a record", Version: toolVersionV1,
		Description:   updateRecordCopy.render(),
		Instead:       updateRecordCopy.Instead,
		RequiredScope: principal.ScopeWrite,
		Tier:          mcp.TierAutoExecute,
		OpenAPIOp:     "updateContact/updateCompany/updateDeal/updateLead/updateActivity/updateProject/updateRelationship/updateProduct/updateOfferTemplate/updateOffer",
		InputSchema: schema(`{"type":"object","required":["record_type","id","fields"],"properties":{
			"record_type":{"type":"string","enum":["contact","company","deal","lead","activity","project","relationship","product","offer_template","offer"]},
			"id":{"type":"string","format":"uuid"},
			"fields":{"type":"object","description":` + jsonString("Only sent fields change. "+recordFieldsDescription) + `},
			"if_version":{"type":"integer","description":"Optimistic-concurrency guard: the last-seen record version"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on retry after a human approved the staged call; send it with exactly the staged arguments"}},
			"additionalProperties":false}`),
		UnkeyedArguments: recordFieldsUnkeyed(),
		OutputSchema:     schemaFor[UpdateResult](),
		UI:               &mcp.ToolUI{ResourceURI: apps.UpdateURI},
	}
}

// StageInfo puts a patch the contract tightened to confirm-first in the inbox
// instead of dead-ending it (#982).
//
// It decodes this door's arguments into the patch command and delegates: the
// refusals and the staged subject live in the resolver (command.go), where the
// REST door reaches the same ones for the same operation.
//
// It stages the whole call. The floor makes this operation confirm-first
// whatever the fields hold, so nothing applies until a human releases it. An
// approved retry carries the ApprovalRedeemed marker.
func (t updateRecord) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args updateRecordArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	// Named field-by-field rather than a bare conversion: PatchCommand
	// deliberately does not carry if_version (command.go's own doc comment
	// says why), so its shape no longer matches updateRecordArgs' and a
	// conversion would not compile.
	return StageSubject(ctx, NewPatchCall(t.p, t.language, PatchCommand{
		RecordType: args.RecordType, ID: args.ID, Fields: args.Fields,
	}))
}

// Handle applies the whole patch. A call a human released through the
// confirm-first floor reaches the same apply, with its approval already spent.
func (t updateRecord) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args updateRecordArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := rejectUnknownFields(updateWriteShapes, args.RecordType, args.Fields); err != nil {
		return nil, err
	}
	return t.apply(ctx, args, args.Fields)
}

// apply writes the patch and answers with the post-write record.
func (t updateRecord) apply(ctx context.Context, args updateRecordArgs, patch json.RawMessage) (json.RawMessage, error) {
	before, rec, err := t.applyRecord(ctx, args, patch)
	if err != nil {
		return nil, err
	}
	return json.Marshal(UpdateResult{
		wireRecord: rec,
		Changes:    t.words.describeUpdate(ctx, summaryIn(ctx, t.language).lang, patch, before.Fields, rec.Fields),
	})
}

// applyRecord writes the patch and reads the record on both sides of it. The
// caller answers with the post-write state (server-derived fields, bumped
// version). It names each change from what it replaced.
func (t updateRecord) applyRecord(ctx context.Context, args updateRecordArgs, patch json.RawMessage) (before datasource.Record, after wireRecord, err error) {
	// Through pinForWrite, not the raw argument. A redeemed retry that supplied
	// no if_version would otherwise write unconditioned: redemption commits its
	// own transaction and this one opens a fresh one, so the skew check inside
	// redemption proves the row was right when the approval was CONSUMED, not
	// when the effect lands — and the agent controls both sides of that window.
	// The REST door forwards the released pin as If-Match, so every operation
	// behind it is already carried; this is the same guarantee on this door.
	pin, err := pinForWrite(ctx, args.IfVersion)
	if err != nil {
		return datasource.Record{}, wireRecord{}, err
	}
	// A null in the patch means clear, read by the rule the REST door uses.
	// The record store clears it or refuses it by name.
	nulls, err := httperr.NullKeysOf(patch)
	if err != nil {
		return datasource.Record{}, wireRecord{}, err
	}
	target := datasource.EntityRef{Type: datasource.EntityType(args.RecordType), ID: args.ID}
	before, err = t.p.Read(ctx, target)
	if err != nil {
		return datasource.Record{}, wireRecord{}, err
	}
	ref, err := t.p.Update(ctx, datasource.UpdateInput{
		Ref:       target,
		Patch:     patch,
		Source:    ToolSource,
		IfVersion: pin,
		Clear:     nulls,
	})
	if err != nil {
		return datasource.Record{}, wireRecord{}, err
	}
	rec, err := t.p.Read(ctx, ref)
	if err != nil {
		return datasource.Record{}, wireRecord{}, fmt.Errorf("crmagents: write landed but read-back failed: %w", err)
	}
	return before, newWireRecord(ctx, rec), nil
}
