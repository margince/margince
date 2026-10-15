// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// switchedModelWindow is how far back a price sync looks for models a provider
// switched calls to.
const switchedModelWindow = 30 * 24 * time.Hour

// reportsServedModel is whether a served identity source is the provider naming
// the model itself. answeredModelSQL and switchedModelSQL test the same grade.
func reportsServedModel(servedIdentitySource string) bool {
	return servedIdentitySource == servedIdentitySourceResponse
}

// gradedServedModel is the model a provider itself named as the one that
// answered, or "" when the trace only echoed or assumed the binding.
func gradedServedModel(servedModel, servedIdentitySource string) string {
	if !reportsServedModel(servedIdentitySource) {
		return ""
	}
	return servedModel
}

// switchedModelSQL is whether the provider named another model than the binding
// as the one that answered the ai_call row aliased alias. A binding that names
// no model, such as the test provider's, has nothing to switch away from.
func switchedModelSQL(alias string) string {
	return fmt.Sprintf(`(%[1]s.served_identity_source = '%[2]s' AND %[1]s.model_id <> '' AND %[1]s.served_model <> '' AND %[1]s.served_model <> %[1]s.model_id)`,
		alias, servedIdentitySourceResponse)
}

// answeredModelSQL is the model that answered the ai_call row aliased alias:
// its graded served model, else the binding's model_id.
func answeredModelSQL(alias string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s.served_identity_source = '%[2]s' AND %[1]s.served_model <> ''
	        THEN %[1]s.served_model ELSE %[1]s.model_id END`, alias, servedIdentitySourceResponse)
}

// switchedServedModels is, by provider, every chat model the provider named as
// answering a call bound to another model since the given instant. Decision
// models are left out: no catalogue the sync reads prices them.
func (s *RateStore) switchedServedModels(ctx context.Context, since time.Time) (map[string][]string, error) {
	out := map[string][]string{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT ac.provider, ac.served_model FROM ai_call ac
			 WHERE ac.occurred_at >= $1 AND ac.kind = '`+callKindCompletion+`' AND `+switchedModelSQL("ac")+`
			 ORDER BY 1, 2`, since)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var provider, served string
			if err := rows.Scan(&provider, &served); err != nil {
				return err
			}
			out[provider] = append(out[provider], served)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("ai: reading the models providers switched to: %w", err)
	}
	return out, nil
}
