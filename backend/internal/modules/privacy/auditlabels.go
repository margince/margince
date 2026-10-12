// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Audit rows named through a port compose fills, since the names live in tables
// this module does not own.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordLabeler names one type's records under the reader's grants. A hidden or
// missing record is absent; an unknown type answers an empty map.
type RecordLabeler interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithRecordLabeler wires the namer. Without one every entity_label reads null.
func (h Handlers) WithRecordLabeler(labeler RecordLabeler) Handlers {
	h.labels = labeler
	return h
}

// labelAuditPage asks once per entity type on the page. A failed read is logged
// and leaves null labels, since the page alone cannot tell lost names from none.
func labelAuditPage(ctx context.Context, labeler RecordLabeler, entries []AuditEntry) {
	if labeler == nil {
		return
	}
	type record struct {
		entityType string
		id         ids.UUID
	}
	seen := map[record]bool{}
	wanted := map[string][]ids.UUID{}
	for _, e := range entries {
		if e.EntityID == nil || seen[record{e.EntityType, *e.EntityID}] {
			continue
		}
		seen[record{e.EntityType, *e.EntityID}] = true
		wanted[e.EntityType] = append(wanted[e.EntityType], *e.EntityID)
	}
	for entityType, want := range wanted {
		labels, err := labeler.Labels(ctx, entityType, want)
		if err != nil {
			slog.ErrorContext(ctx, "privacy: audit entries could not be named; their entity_label reads null",
				"entity_type", entityType, "records", len(want), "err", err)
			continue
		}
		for i := range entries {
			if entries[i].EntityType != entityType || entries[i].EntityID == nil {
				continue
			}
			if label, ok := labels[*entries[i].EntityID]; ok && label != "" {
				entries[i].EntityLabel = &label
			}
		}
	}
}
