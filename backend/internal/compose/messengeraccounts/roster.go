// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messengeraccounts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// RecordRoster reads CRM assignment for native-audience comparison.
func RecordRoster(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID, owner *ids.UUID) ([]chatlinks.RecordPerson, error) {
	columns := map[string]string{"company": "company_id", "deal": "deal_id", "project": "project_id"}
	column, ok := columns[kind]
	if !ok {
		return nil, apperrors.ErrInvalidArgument
	}
	if err := auth.EnsureVisibleExisting(ctx, tx, kind, id); err != nil {
		return nil, err
	}
	assigned := `SELECT a.user_id,a.team_id FROM record_assignment a WHERE a.` + pgx.Identifier{column}.Sanitize() + `=@record AND a.archived_at IS NULL`
	// Empty or retired assigned teams remain visible as unresolved recipients.
	query := `WITH assigned AS (` + assigned + `)
 SELECT u.id,'user',u.display_name,u.email,(` + workforce.LiveHumanSQL("u") + `)
 FROM app_user u WHERE NOT u.is_agent AND (u.id=@owner OR EXISTS(
  SELECT 1 FROM assigned a WHERE a.user_id=u.id OR EXISTS(
   SELECT 1 FROM team_membership m JOIN team t ON t.id=m.team_id AND t.archived_at IS NULL
   WHERE m.team_id=a.team_id AND m.user_id=u.id)))
 UNION ALL
 SELECT t.id,'team',t.name,'',false FROM team t WHERE EXISTS(
  SELECT 1 FROM assigned a WHERE a.team_id=t.id) AND (t.archived_at IS NOT NULL OR NOT EXISTS(
  SELECT 1 FROM team_membership m JOIN app_user u ON u.id=m.user_id AND NOT u.is_agent WHERE m.team_id=t.id))
 ORDER BY 3,1 LIMIT 1001`
	rows, err := tx.Query(ctx, query, pgx.StrictNamedArgs{"record": id, "owner": owner})
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chatlinks.RecordPerson{}
	for rows.Next() {
		var person chatlinks.RecordPerson
		if err := rows.Scan(&person.ID, &person.Kind, &person.Name, &person.Email, &person.Active); err != nil {
			return nil, err
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 1000 {
		return nil, messaging.ErrUnsupported
	}
	return out, nil
}
