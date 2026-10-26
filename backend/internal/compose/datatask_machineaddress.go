// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// machineAddressTag is the word existing machine-address contacts are filed
// under, for a user to find with the tag filter and archive or keep. Nothing
// is archived or deleted by the task.
const machineAddressTag = "machine-address"

// machineAddressTaggedBy keys the run id on each audit row a data task writes.
const machineAddressTaggedBy = "data_task_run"

func machineAddressDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792645085_machine_address_contacts", Version: 2,
		Title: "Tag contacts made from a sending system's address", Requires: []string{"core/1788327300"},
		Steps: []datatask.Step{{
			Name: "tag_machine_address_contacts", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMachineAddressContacts, Batch: tagMachineAddressContactsBatch,
		}},
	}
}

func tagMachineAddressContactsBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := contacts.MachineAddressContactsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed, err := collections.ApplySystemTagTx(ctx, tx, machineAddressTag, "contact", page.Machine,
		map[string]any{machineAddressTaggedBy: in.RunID.String()})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, changed)
}

func estimateMachineAddressContacts(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountContactsWithAddress(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
