// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TeamInstallationByNative resolves a verified native team under its application and tenant.
func (s *Store) TeamInstallationByNative(ctx context.Context, app, tenant, native string) (TeamInstallation, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return TeamInstallation{}, err
	}
	var out TeamInstallation
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+teamInstallationColumns+` FROM chat_team_installation WHERE app_id=@app AND tenant_id=@tenant AND native_team_id=@native`, pgx.StrictNamedArgs{appKey: app, tenantKey: tenant, "native": native})
		if err != nil {
			return err
		}
		out, err = pgx.CollectOneRow(rows, scanTeamInstallation)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperrors.ErrNotFound
	}
	return out, err
}

// TeamThread resolves delivery addressing from verified thread evidence.
func (s *Store) TeamThread(ctx context.Context, app, tenant, channel, root string) (teams.ConversationReference, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return teams.ConversationReference{}, err
	}
	team, _, ok := strings.Cut(channel, "/")
	if !ok {
		return teams.ConversationReference{}, apperrors.ErrInvalidArgument
	}
	ref := teams.ConversationReference{App: app, Tenant: tenant, Channel: channel, Root: root, Activity: root}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT t.conversation_id FROM chat_team_thread t JOIN chat_team_installation i ON i.id=t.installation_id WHERE i.app_id=@app AND i.tenant_id=@tenant AND i.team_id=@team AND i.installed AND t.channel_id=@channel AND t.root_id=@root`, pgx.StrictNamedArgs{appKey: app, tenantKey: tenant, teamKey: team, channelKey: channel, rootKey: root}).Scan(&ref.Conversation)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ref, apperrors.ErrNotFound
	}
	return ref, err
}

// RecordTeamThread binds a native thread to its verified Bot Framework conversation.
func (s *Store) RecordTeamThread(ctx context.Context, installation TeamInstallation, channel, root, conversation string, at time.Time) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if !validTeamThread(installation, channel, root, conversation, at) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var admitted bool
		if err := tx.QueryRow(ctx, `SELECT installed AND signed_at<=@at FROM chat_team_installation WHERE id=@id AND app_id=@app AND tenant_id=@tenant AND team_id=@team FOR SHARE`, pgx.StrictNamedArgs{"id": installation.ID, "app": installation.AppID, tenantKey: installation.Tenant, "team": installation.Team, "at": at}).Scan(&admitted); err != nil {
			return err
		}
		if !admitted {
			return apperrors.ErrPermissionDenied
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "teams-thread:" + installation.ID.String() + ":" + channel + ":" + root}); err != nil {
			return err
		}
		var id ids.UUID
		var priorConversation string
		var signed time.Time
		err := tx.QueryRow(ctx, `SELECT id,conversation_id,signed_at FROM chat_team_thread WHERE installation_id=@installation AND channel_id=@channel AND root_id=@root FOR UPDATE`, pgx.StrictNamedArgs{"installation": installation.ID, channelKey: channel, rootKey: root}).Scan(&id, &priorConversation, &signed)
		operation := updateKey
		var before map[string]any
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			operation = ModeCreate
			id = ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO chat_team_thread(id,installation_id,channel_id,root_id,conversation_id,signed_at) VALUES(@id,@installation,@channel,@root,@conversation,@at)`, pgx.StrictNamedArgs{"id": id, "installation": installation.ID, "channel": channel, rootKey: root, conversationKey: conversation, "at": at}); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if !at.After(signed) {
				return nil
			}
			before = map[string]any{channelIDKey: channel, rootIDKey: root, conversationIDKey: priorConversation, "signed_at": signed}
			if _, err := tx.Exec(ctx, `UPDATE chat_team_thread SET conversation_id=@conversation,signed_at=@at,observed_at=now() WHERE id=@id`, pgx.StrictNamedArgs{"id": id, "conversation": conversation, "at": at}); err != nil {
				return err
			}
		}
		_, err = storekit.Audit(ctx, tx, operation, "chat_team_thread", id, before, map[string]any{"channel_id": channel, "root_id": root, "conversation_id": conversation, "signed_at": at})
		return err
	})
}

func validTeamThread(installation TeamInstallation, channel, root, conversation string, at time.Time) bool {
	team, _, ok := strings.Cut(channel, "/")
	if !ok || team != installation.Team || root == "" || conversation == "" || len(channel) > 2048 || len(root) > 1024 || len(conversation) > 4096 || at.IsZero() {
		return false
	}
	return true
}
