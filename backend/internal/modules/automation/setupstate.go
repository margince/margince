// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

type configurationState struct {
	State  crmcontracts.AutomationSetupState
	Reason string
}

func configurationOf(a Automation, authorizerActive bool) configurationState {
	entry, known := CatalogEntryByKey(a.Key)
	var params map[string]any
	if !known || json.Unmarshal(a.Params, &params) != nil || entry.Validate(params) != nil || validateRequiredSetup(entry, params) != nil {
		return configurationState{crmcontracts.AutomationSetupStateNeedsSetup, "configuration_incomplete"}
	}
	needsAuthorizer, err := starterNeedsAuthorizer(entry, a.Params)
	if err != nil {
		return configurationState{crmcontracts.AutomationSetupStateNeedsSetup, "configuration_incomplete"}
	}
	if needsAuthorizer && a.OwnerID == nil {
		return configurationState{crmcontracts.AutomationSetupStateNeedsSetup, "authorizer_missing"}
	}
	if a.OwnerID != nil && !authorizerActive {
		return configurationState{crmcontracts.AutomationSetupStateNeedsAttention, "authorizer_inactive"}
	}
	if a.PausedReason != nil {
		return configurationState{crmcontracts.AutomationSetupStateNeedsAttention, "paused_by_system"}
	}
	if a.Enabled {
		return configurationState{State: crmcontracts.AutomationSetupStateOn}
	}
	return configurationState{State: crmcontracts.AutomationSetupStatePaused}
}

func (s *AutomationStore) readConfigurationStates(ctx context.Context, records []Automation) error {
	var owners []ids.UUID
	for _, record := range records {
		if record.OwnerID != nil {
			owners = append(owners, record.OwnerID.UUID)
		}
	}
	active := map[ids.UUID]bool{}
	if len(owners) > 0 {
		if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT u.id, `+workforce.LiveHumanSQL("u")+` FROM app_user u WHERE u.id=ANY(@authorizers)`, pgx.NamedArgs{"authorizers": owners})
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id ids.UUID
				var live bool
				if err := rows.Scan(&id, &live); err != nil {
					return err
				}
				active[id] = live
			}
			return rows.Err()
		}); err != nil {
			return err
		}
	}
	for index, record := range records {
		live := false
		if record.OwnerID != nil {
			live = active[record.OwnerID.UUID]
		}
		state := configurationOf(record, live)
		records[index].Configuration = &state
	}
	return nil
}
