// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// claimStarterAuthorizer gives a reviewed template the enabling contact's
// authority. Existing owners never change when somebody toggles the template.
func claimStarterAuthorizer(ctx context.Context, tx pgx.Tx, before Automation) (ids.UUID, error) {
	entry, ok := CatalogEntryByKey(before.Key)
	if !ok {
		return ids.Nil, fmt.Errorf("automation %s names an unknown catalog key", before.ID)
	}
	needed, err := starterNeedsAuthorizer(entry, before.Params)
	if err != nil {
		return ids.Nil, err
	}
	if !needed {
		return ids.Nil, nil
	}
	if err := requireAuthorCeiling(ctx, entry); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.Nil, &MissingDraftOwnerError{}
	}
	updated, err := tx.Exec(ctx, `UPDATE automation SET owner_id = @owner
		WHERE id = @id AND owner_id IS NULL`,
		pgx.NamedArgs{"owner": actor.UserID, "id": before.ID})
	if err != nil || updated.RowsAffected() == 0 {
		return ids.Nil, err
	}
	return actor.UserID, nil
}

func starterNeedsAuthorizer(entry CatalogEntry, params json.RawMessage) (bool, error) {
	if entry.Action == string(ActionTypeDraftEmail) {
		return true, nil
	}
	if _, quiet := activityScanHandlers[entry.Key]; quiet {
		population, err := quietPopulation(params)
		return population.configured(), err
	}
	if entry.Key == stageChangeNotifyName || entry.Key == stageChangeCreateTaskName {
		var scope stageChangeParams
		err := json.Unmarshal(params, &scope)
		return !scope.PipelineID.IsZero(), err
	}
	return false, nil
}
