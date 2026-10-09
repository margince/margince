// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What each audit row is about, by name. The names live in tables this module
// does not own, so each owning module answers for its own rows and visibility
// through a port compose fills.

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordLabeler names a set of one entity type's records under the reader's
// grants. A record the reader may not see, or that is gone, is absent, and a
// type it does not name answers an empty map.
type RecordLabeler interface {
	Labels(ctx context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error)
}

// WithRecordLabeler wires the namer. Without one every entity_label reads null.
func (h Handlers) WithRecordLabeler(labeler RecordLabeler) Handlers {
	h.labels = labeler
	return h
}

// labelAuditPage asks once per entity type on the page, with that type's
// distinct ids, so the cost follows the types present and not the row count.
//
// A type whose read fails keeps null labels and the page still answers; the
// failure is logged because a page that quietly lost its names reads as a log
// of nameless records.
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
