// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordLabel pulls a human-readable name out of a record's fields for
// inbox summaries; falls back to the id.
func recordLabel(rec datasource.Record) string {
	var f struct {
		FullName    string `json:"full_name"`
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		Kind        string `json:"kind"`
		Subject     string `json:"subject"`
	}
	if err := json.Unmarshal(rec.Fields, &f); err != nil {
		return rec.Ref.ID.String()
	}
	// Relationships and activities need the label that identifies the human decision.
	if rec.Ref.Type == datasource.EntityRelationship && f.Kind != "" {
		return fmt.Sprintf("%q", f.Kind)
	}
	if rec.Ref.Type == datasource.EntityActivity && f.Subject != "" {
		return fmt.Sprintf("%q", f.Subject)
	}
	for _, s := range []string{f.FullName, f.DisplayName, f.Name, f.Email} {
		if s != "" {
			return fmt.Sprintf("%q", s)
		}
	}
	return rec.Ref.ID.String()
}
