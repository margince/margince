// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

const mergedAccounts = `CASE WHEN $1::boolean THEN (
 WITH RECURSIVE prior_accounts(id, display_name) AS (
  SELECT retired.id, retired.display_name FROM app_user retired
  WHERE retired.merged_into_user_id = app_user.id
  UNION SELECT retired.id, retired.display_name FROM app_user retired
  JOIN prior_accounts prior ON retired.merged_into_user_id = prior.id)
 SELECT COALESCE(jsonb_agg(jsonb_build_object('id', id, 'display_name', display_name, 'departed', true) ORDER BY display_name, id), '[]'::jsonb)
 FROM prior_accounts) ELSE NULL::jsonb END`

func rowIDs(rows []userRow) []ids.UUID {
	out := make([]ids.UUID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ID)
	}
	return out
}
