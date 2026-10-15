// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "fmt"

// gradedServedModel is the model a provider itself named as the one that
// answered, or "" when the trace only echoed or assumed the binding. Only a
// "response" grade counts, here and in answeredModelSQL alike.
func gradedServedModel(servedModel, servedIdentitySource string) string {
	if servedIdentitySource != servedIdentitySourceResponse {
		return ""
	}
	return servedModel
}

// answeredModelSQL is the model that answered the ai_call row aliased alias:
// its graded served model, else the binding's model_id.
func answeredModelSQL(alias string) string {
	return fmt.Sprintf(`CASE WHEN %[1]s.served_identity_source = '%[2]s' AND %[1]s.served_model <> ''
	        THEN %[1]s.served_model ELSE %[1]s.model_id END`, alias, servedIdentitySourceResponse)
}
