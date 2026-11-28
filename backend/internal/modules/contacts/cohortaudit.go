// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

const (
	cohortLinkedKey   = "cohort_linked"
	cohortPromotedKey = "cohort_promoted"
)

// CohortAuditSQL identifies the additive address-to-contact repair by its immutable audit payload.
func CohortAuditSQL(alias string) string {
	return `COALESCE((` + alias + `.entity_type='contact' AND ` + alias + `.action='update' AND ` + alias + `.after ? '` + cohortLinkedKey + `' AND ` + alias + `.after ? '` + cohortPromotedKey + `'),false)`
}

// A relink and a participant promotion can touch the same activity.
// Publish once, including participant-only repairs that change graph edges.
func changedActivities(linked, promoted []ids.UUID) []ids.UUID {
	seen := make(map[ids.UUID]struct{}, len(linked)+len(promoted))
	out := make([]ids.UUID, 0, len(linked)+len(promoted))
	for _, group := range [][]ids.UUID{linked, promoted} {
		for _, id := range group {
			if _, already := seen[id]; already {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
