// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

type configurationState struct{ State, Reason string }

func configurationOf(a Automation, authorizerActive bool) configurationState {
	entry, known := CatalogEntryByKey(a.Key)
	var params map[string]any
	if !known || json.Unmarshal(a.Params, &params) != nil || entry.Validate(params) != nil || validateRequiredSetup(entry, params) != nil {
		return configurationState{"needs_setup", "configuration_incomplete"}
	}
	needsAuthorizer, err := starterNeedsAuthorizer(entry, a.Params)
	if err != nil {
		return configurationState{"needs_setup", "configuration_incomplete"}
	}
	if needsAuthorizer && a.OwnerID == nil {
		return configurationState{"needs_setup", "authorizer_missing"}
	}
	if a.OwnerID != nil && !authorizerActive {
		return configurationState{"needs_attention", "authorizer_inactive"}
	}
	if a.PausedReason != nil {
		return configurationState{"needs_attention", "paused_by_system"}
	}
	if a.Enabled {
		return configurationState{State: "on"}
	}
	return configurationState{State: "paused"}
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
			rows, err := tx.Query(ctx, `SELECT u.id, `+workforce.LiveHumanSQL("u")+` FROM app_user u WHERE u.id=ANY(@owners)`, pgx.NamedArgs{"owners": owners})
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
