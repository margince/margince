// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentUsageObject gates the per-user usage read. It is the AI usage page's
// own grant, because the card that shows it sits on that page.
const agentUsageObject = "ai_diagnostics"

// AgentUsageHolder is one user with live agent credentials, and the counter
// subjects those credentials are charged to. A connection's passports share
// one subject, so it is listed once.
type AgentUsageHolder struct {
	UserID      ids.UUID
	DisplayName string
	Subjects    []string
}

// AgentUsageHolders lists every user with a live passport or connection.
// It reads passports across people, so it answers only how many and whose;
// the handler adds how much and nothing else.
func (s *Service) AgentUsageHolders(ctx context.Context, id Identity) ([]AgentUsageHolder, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	ctx = actorCtx(ctx, id)
	if err := auth.Require(ctx, agentUsageObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []AgentUsageHolder
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Live by the rule authentication applies, so a passport that could not
		// make a call is not counted as one that could.
		rows, err := tx.Query(ctx, `
			SELECT u.id, u.display_name, p.id, p.oauth_grant_id
			FROM passport p
			JOIN app_user u ON u.id = p.on_behalf_of`+agentLivenessJoins+`
			WHERE true`+agentLivenessWhere+`
			ORDER BY u.display_name, u.id, p.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				user       ids.UUID
				name       string
				passport   ids.UUID
				connection *ids.UUID
			)
			if err := rows.Scan(&user, &name, &passport, &connection); err != nil {
				return err
			}
			var grant ids.UUID
			if connection != nil {
				grant = *connection
			}
			out = addHolderSubject(out, user, name, agentvolume.SubjectOf(passport, grant))
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// addHolderSubject appends subject to the user it belongs to. Rows arrive
// grouped by user, so only the last holder can be theirs.
func addHolderSubject(holders []AgentUsageHolder, user ids.UUID, name, subject string) []AgentUsageHolder {
	if n := len(holders); n > 0 && holders[n-1].UserID == user {
		for _, known := range holders[n-1].Subjects {
			if known == subject {
				return holders
			}
		}
		holders[n-1].Subjects = append(holders[n-1].Subjects, subject)
		return holders
	}
	return append(holders, AgentUsageHolder{UserID: user, DisplayName: name, Subjects: []string{subject}})
}
