// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WorkspaceHistoryProfile preserves the declared Slack limits without requiring a shared bot.
func (s *Store) WorkspaceHistoryProfile(ctx context.Context, provider string) (*chatwire.HistoryProfile, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if provider != ProviderSlack {
		return nil, nil //nolint:nilnil // Other services have no Slack quota profile.
	}
	var raw []byte
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT (SELECT slack_history_profile FROM chat_service WHERE singleton)").Scan(&raw)
	})
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var profile chatwire.HistoryProfile
	if json.Unmarshal(raw, &profile) != nil {
		return nil, apperrors.ErrInvalidArgument
	}
	return &profile, nil
}

// SetWorkspaceHistoryProfile fences quota changes against a later service selection.
func (s *Store) SetWorkspaceHistoryProfile(ctx context.Context, selected teamchat.Selection, distribution string, at time.Time) error {
	if _, err := memberMutationActor(ctx); err != nil {
		return err
	}
	if selected.Provider != ProviderSlack || selected.Version == 0 || at.IsZero() {
		return apperrors.ErrInvalidArgument
	}
	profile, err := chatwire.SlackHistoryProfile(distribution, at)
	if err != nil {
		return apperrors.ErrInvalidArgument
	}
	raw, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if auth.Require(ctx, "channel_connection", principal.ActionDelete) != nil {
			if err := auth.Require(ctx, "channel_connection", principal.ActionCreate); err != nil {
				return err
			}
		}
		if err := teamchat.HoldSelection(ctx, tx, selected); err != nil {
			return err
		}
		var id ids.UUID
		var before []byte
		if err := tx.QueryRow(ctx, "SELECT id,slack_history_profile FROM chat_service WHERE singleton FOR UPDATE").Scan(&id, &before); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE chat_service SET slack_history_profile=@profile,updated_at=now() WHERE singleton", pgx.StrictNamedArgs{"profile": raw}); err != nil {
			return err
		}
		type profileImage struct {
			Profile json.RawMessage `json:"slack_history_profile"`
		}
		audit, err := storekit.Audit(ctx, tx, updateKey, "chat_service", id, profileImage{Profile: before}, profileImage{Profile: raw})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.InternalEventChatServiceSelected{})
	})
}
