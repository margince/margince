// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// heldRecord is the rule that keeps one kind of record: its table aliased a,
// and the predicate over that row. A predicate that reads the floor takes its
// current and carried classes as the statement's first two arguments.
type heldRecord struct {
	from      string
	predicate string
	floor     bool
}

// args puts the floor's classes first when the predicate reads them.
//
//craft:ignore naked-any the statement's arguments are one record id or a slice of them, bound by pgx
func (r heldRecord) args(floor FloorPolicy, records any) []any {
	if !r.floor {
		return []any{records}
	}
	current, carried := floor.sqlArgs()
	return []any{current, carried, records}
}

// heldRecordRule is the legal hold, open dispute or statutory floor that keeps
// a record of kind, or false for a kind no hold reaches. The audit cleanup asks
// it of one record and the copy retention of a page of them, so the two keep
// the same records.
func heldRecordRule(kind string) (heldRecord, bool) {
	switch kind {
	case contactObject:
		return heldRecord{from: " FROM contact a", predicate: "a.legal_hold"}, true
	case "lead":
		return heldRecord{from: " FROM lead a", predicate: "a.legal_hold"}, true
	case reportingDeal:
		return heldRecord{from: " FROM deal a", predicate: "a.legal_hold"}, true
	case "company":
		return heldRecord{from: " FROM company a", predicate: "a.legal_hold"}, true
	case "project":
		return heldRecord{from: " FROM project a", predicate: "a.legal_hold"}, true
	case rightsRequestEntity:
		return heldRecord{from: " FROM data_subject_request a", predicate: "a.dispute_open"}, true
	case "activity":
		return heldRecord{from: " FROM activity a", predicate: "a.restricted_at IS NOT NULL OR " +
			retentionscope.HeldThroughAnyLink("a.id") + " OR (" + handelsbriefShielded(1, 2) + ")", floor: true}, true
	}
	return heldRecord{}, false
}

// RecordCopy is a stored copy of a record's content and the record it was made
// from. A published event and a rendered webhook body are copies.
type RecordCopy struct {
	ID         ids.UUID
	EntityType string
	EntityID   ids.UUID
}

// heldCopies answers which copies a hold or floor keeps: those whose record
// heldRecordRule keeps under floor.
func heldCopies(ctx context.Context, tx pgx.Tx, copies []RecordCopy, floor FloorPolicy) (map[ids.UUID]bool, error) {
	byKind := map[string][]ids.UUID{}
	for _, c := range copies {
		byKind[c.EntityType] = append(byKind[c.EntityType], c.EntityID)
	}
	held := map[ids.UUID]bool{}
	for kind, records := range byKind {
		rule, ok := heldRecordRule(kind)
		if !ok {
			continue
		}
		args := rule.args(floor, records)
		rows, err := tx.Query(ctx, `SELECT a.id`+rule.from+` WHERE a.id = ANY(`+
			storekit.SQLf("$%d", len(args))+`) AND (`+rule.predicate+`)`, args...)
		if err != nil {
			return nil, fmt.Errorf("privacy: reading which %s records are held: %w", kind, err)
		}
		kept, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, err
		}
		for _, record := range kept {
			held[record] = true
		}
	}
	out := map[ids.UUID]bool{}
	for _, c := range copies {
		if held[c.EntityID] {
			out[c.ID] = true
		}
	}
	return out, nil
}
