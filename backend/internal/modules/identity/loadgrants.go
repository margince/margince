// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What a seat may DO, loaded once per authentication.
//
// Split out of service.go, which held both the session lifecycle — login,
// authenticate, logout — and the grant resolution those three each end by
// calling. They are different subjects: one is about proving who is asking, the
// other about what the answer entitles them to, and the file had grown past the
// length cap holding both.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func loadGrants(ctx context.Context, tx pgx.Tx, userID ids.UserID) (roles []string, teams []ids.TeamID, perms principal.Permissions, err error) {
	rows, err := tx.Query(ctx,
		`SELECT r.key, r.permissions FROM role_assignment ra JOIN role r ON r.id = ra.role_id WHERE ra.user_id = $1`, userID)
	if err != nil {
		return nil, nil, principal.Permissions{}, err
	}
	defer rows.Close()
	byRole := map[string]policy.Document{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, nil, principal.Permissions{}, err
		}
		doc, err := policy.Parse(raw)
		if err != nil {
			// A role carrying an UNREADABLE policy document is a data defect
			// the login must surface, not silently downgrade to no access.
			//
			// "Unreadable" is now a much narrower set than it was: malformed
			// JSON, or a row_scope nothing can interpret. An object this
			// installation does not know is dropped by Parse with a log line
			// instead of failing here — because failing here failed the whole
			// LOGIN, so removing a composed extension locked out every user
			// whose role still carried its object (Task 14 UAT, F4).
			return nil, nil, principal.Permissions{}, fmt.Errorf("crmauth: role %q: %w", key, err)
		}
		roles = append(roles, key)
		byRole[key] = doc
	}
	if err := rows.Err(); err != nil {
		return nil, nil, principal.Permissions{}, err
	}

	// Live teams only: an archived team keeps its membership rows so a
	// restore brings them back, but while archived it resolves neither row
	// scope nor a team share.
	teamRows, err := tx.Query(ctx,
		`SELECT tm.team_id FROM team_membership tm JOIN team t ON t.id = tm.team_id AND t.archived_at IS NULL
		  WHERE tm.user_id = $1`, userID)
	if err != nil {
		return nil, nil, principal.Permissions{}, err
	}
	defer teamRows.Close()
	for teamRows.Next() {
		var t ids.TeamID
		if err := teamRows.Scan(&t); err != nil {
			return nil, nil, principal.Permissions{}, err
		}
		teams = append(teams, t)
	}
	if err := teamRows.Err(); err != nil {
		return nil, nil, principal.Permissions{}, err
	}
	perms = policy.Merge(byRole)
	perms.FieldMasks, err = loadFieldMasks(ctx, tx, roles)
	return roles, teams, perms, err
}

// rawTeamIDs widens typed team ids to the untyped []ids.UUID the kernel
// principal and the authz port carry — the row-scope seams stay untyped
// (they compare team membership against polymorphic scope clauses).
func rawTeamIDs(teams []ids.TeamID) []ids.UUID {
	if teams == nil {
		return nil
	}
	out := make([]ids.UUID, len(teams))
	for i, t := range teams {
		out[i] = t.UUID
	}
	return out
}
