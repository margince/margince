// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamInstallation pins native team evidence to the application that received it.
type TeamInstallation struct {
	ID         ids.UUID
	AppID      string
	Tenant     string
	Team       string
	NativeTeam string
	BotMember  string
	ServiceURL string
	Name       string
	Installed  bool
	SignedAt   time.Time
}

const teamInstallationColumns = `id,app_id,tenant_id,team_id,native_team_id,bot_member_id,service_url,team_name,installed,signed_at`

func scanTeamInstallation(row pgx.CollectableRow) (TeamInstallation, error) {
	var out TeamInstallation
	err := row.Scan(&out.ID, &out.AppID, &out.Tenant, &out.Team, &out.NativeTeam, &out.BotMember, &out.ServiceURL, &out.Name, &out.Installed, &out.SignedAt)
	return out, err
}

// TeamInstallations lists verified installations for the configured application and tenant.
func (s *Store) TeamInstallations(ctx context.Context, app, tenant string) ([]TeamInstallation, error) {
	if err := auth.Require(ctx, "channel_connection", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []TeamInstallation
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+teamInstallationColumns+` FROM chat_team_installation WHERE app_id=@app AND tenant_id=@tenant AND installed ORDER BY team_name,id`, pgx.StrictNamedArgs{appKey: app, tenantKey: tenant})
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, scanTeamInstallation)
		return err
	})
	return out, err
}

// RecordTeamInstallation persists installation evidence received from a verified Teams activity.
func (s *Store) RecordTeamInstallation(ctx context.Context, in TeamInstallation) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	for _, id := range []string{in.AppID, in.Tenant, in.Team} {
		if _, err := ids.Parse(id); err != nil {
			return apperrors.ErrInvalidArgument
		}
	}
	if err := teams.CheckServiceURL(in.ServiceURL); err != nil {
		return apperrors.ErrInvalidArgument
	}
	if !validTeamInstallation(in) {
		return apperrors.ErrInvalidArgument
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended(@key,0))`, pgx.StrictNamedArgs{keyKey: "teams-installation:" + in.AppID + ":" + in.Tenant + ":" + in.Team}); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+teamInstallationColumns+` FROM chat_team_installation WHERE app_id=@app AND tenant_id=@tenant AND team_id=@team FOR UPDATE`, pgx.StrictNamedArgs{appKey: in.AppID, tenantKey: in.Tenant, teamKey: in.Team})
		if err != nil {
			return err
		}
		before, err := pgx.CollectOneRow(rows, scanTeamInstallation)
		operation := updateKey
		var beforeImage map[string]any
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			operation = ModeCreate
			in.ID = ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO chat_team_installation(id,app_id,tenant_id,team_id,native_team_id,bot_member_id,service_url,team_name,installed,signed_at) VALUES(@id,@app,@tenant,@team,@native,@bot,@service,@name,@installed,@signed)`, teamInstallationArgs(in)); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if !in.SignedAt.After(before.SignedAt) {
				return nil
			}
			in.ID = before.ID
			beforeImage = teamInstallationImage(before)
			args := teamInstallationArgs(in)
			delete(args, "app")
			delete(args, tenantKey)
			delete(args, "team")
			if _, err := tx.Exec(ctx, `UPDATE chat_team_installation SET native_team_id=@native,bot_member_id=@bot,service_url=@service,team_name=@name,installed=@installed,signed_at=@signed,observed_at=now() WHERE id=@id`, args); err != nil {
				return err
			}
		}
		_, err = storekit.Audit(ctx, tx, operation, "chat_team_installation", in.ID, beforeImage, teamInstallationImage(in))
		return err
	})
}

func teamInstallationArgs(in TeamInstallation) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{"id": in.ID, "app": in.AppID, tenantKey: in.Tenant, teamKey: in.Team, "native": in.NativeTeam, "bot": in.BotMember, "service": in.ServiceURL, "name": in.Name, "installed": in.Installed, "signed": in.SignedAt}
}

func teamInstallationImage(in TeamInstallation) map[string]any {
	return map[string]any{"app_id": in.AppID, "tenant_id": in.Tenant, "team_id": in.Team, "native_team_id": in.NativeTeam, "bot_member_id": in.BotMember, "service_url": in.ServiceURL, "team_name": in.Name, "installed": in.Installed, signedAtKey: in.SignedAt}
}

func validTeamInstallation(in TeamInstallation) bool {
	return in.NativeTeam != "" && in.BotMember != "" && len(in.NativeTeam) <= 1024 && len(in.BotMember) <= 1024 && len(in.Name) <= 500 && !in.SignedAt.IsZero()
}
