// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tool door onto the bulk-change engine. bulk_update_records and the
// /v1/bulk routes run the same engine, so a change refused, excluded or
// confirmed on one door is refused, excluded or confirmed the same way on the
// other.

import (
	"context"
	"encoding/json"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type bulkChangeSeam struct{ engine *bulkEngine }

func (s bulkChangeSeam) PreviewBulkChange(ctx context.Context, cmd agents.BulkChangeCommand) (json.RawMessage, error) {
	out, err := s.engine.Preview(ctx, bulkChangeFromCommand(cmd))
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (s bulkChangeSeam) ExecuteBulkChange(ctx context.Context, cmd agents.BulkChangeCommand) (json.RawMessage, error) {
	out, err := s.engine.Execute(ctx, bulkChangeFromCommand(cmd))
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (s bulkChangeSeam) PreviewBulkUndo(ctx context.Context, batchID ids.UUID) (json.RawMessage, error) {
	out, err := s.engine.PreviewUndo(ctx, batchID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (s bulkChangeSeam) UndoBulkChange(ctx context.Context, batchID ids.UUID, confirmToken string) (json.RawMessage, error) {
	out, err := s.engine.Undo(ctx, batchID, confirmToken)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func bulkChangeFromCommand(cmd agents.BulkChangeCommand) bulkChange {
	items := make([]crmcontracts.BulkItem, len(cmd.Items))
	for i, item := range cmd.Items {
		items[i] = crmcontracts.BulkItem{Id: openapi_types.UUID(item.ID), Version: item.Version}
	}
	return bulkChange{
		recordType:   crmcontracts.BulkRecordType(cmd.RecordType),
		verb:         crmcontracts.BulkVerb(cmd.Verb),
		items:        items,
		ownerID:      cmd.OwnerID,
		listID:       cmd.ListID,
		note:         cmd.Note,
		confirmToken: cmd.ConfirmToken,
	}
}

// bulkChangeCommand decodes the bulk routes for the REST door's governance
// seam. Only the size is read, and an undo's body names none: the command is
// asked for a staged subject, and a bulk change or its undo refuses to stage
// at all.
//
//nolint:ireturn // a decoder's whole product is the erased command-and-resolver pair restCommands is typed by
func bulkChangeCommand(_ agentPolicy, deps restCommandDeps, _ *http.Request, body []byte) (agents.GovernedCall, error) {
	var in struct {
		Items []agents.BulkItem `json:"items"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &in); err != nil {
			return nil, err
		}
	}
	return agents.NewBulkChangeCall(deps.language, agents.BulkChangeCommand{Items: in.Items}), nil
}
