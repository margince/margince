// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a change has been overtaken.
//
// The question is whether the field's value has MOVED since this entry left it,
// not whether anybody wrote it. Those differ, and the difference is what makes
// undoing several changes in a row work at all.
//
// A -> B -> C, reverted C -> B -> A. Undoing the C entry writes B and records a
// reversal row. If supersession counted writes, that reversal row would be "a
// later write" of the same field and the B entry would refuse — so walking back
// through a record's history could never get past the first step. Asking about
// the VALUE instead: after undoing C the field holds B, which is exactly what
// the B entry left it at, so the B entry is undoable and the walk continues.
//
// It also refuses the case that matters. If somebody else changed the field and
// their change is still standing, the value is theirs rather than this entry's,
// and putting this entry back would take their decision with it. That is the
// ambiguity the product must name rather than resolve.
//
// The cost, stated plainly: a colleague who re-typed the SAME value has
// re-affirmed it, and this cannot see that — the value did not move, so the
// undo is allowed. The earlier rule caught it, at the price of making
// sequential undo impossible. Between refusing a re-affirmation nobody can
// distinguish from a no-op and refusing every second undo, this is the better
// trade, and it is a trade rather than a free win.
//
// This is deliberately NOT HumanOwnedConflicts (humanprecedence.go), which asks
// which keys a HUMAN last wrote to a different value, with no cutoff, to decide
// whether an AGENT's write needs approval. Sharing a reader would either weaken
// that guardrail or reimpose the write-counting rule here.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

// moneyPair is read as ONE field for supersession. amount_minor is a count of
// units the currency defines, so a later change to either makes a restore of
// the other state a value that never existed, wrong by the scale difference
// values.MinorUnitExceptions() encodes — and wrong silently, because the number
// is plausible in both denominations.
//
//nolint:goconst // the pair IS the declaration; a constant for either half would read as if one were special
var moneyPair = []string{"amount_minor", "currency"}

// reportedAs maps a superseded key back onto the keys the caller asked about.
// The money pair travels together in both directions: a later change to the
// currency supersedes a restore of the amount, and the caller hears it named as
// the field it asked to put back.
func reportedAs(superseded []string, asked []string) []string {
	wasAsked := make(map[string]bool, len(asked))
	for _, key := range asked {
		wasAsked[key] = true
	}
	hit := make(map[string]bool, len(superseded))
	moneyMoved := false
	for _, key := range superseded {
		if wasAsked[key] {
			hit[key] = true
		}
		for _, half := range moneyPair {
			if key == half {
				moneyMoved = true
			}
		}
	}
	if moneyMoved {
		for _, half := range moneyPair {
			if wasAsked[half] {
				hit[half] = true
			}
		}
	}
	out := make([]string, 0, len(hit))
	for key := range hit {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// fieldsThatMovedSince names the keys whose value on the record is no longer
// what this entry left them at. It takes the caller's transaction so the
// binding evaluation reads the same row the write is about to take.
//
// The comparison is on jsonb, through the same representation the image was
// written in, rather than a per-type Go conversion that would disagree about
// dates and money.
func fieldsThatMovedSince(ctx context.Context, tx pgx.Tx, row AuditRow) ([]string, error) {
	after, entityType, id := row.After, row.EntityType, row.EntityID
	if len(after) == 0 {
		return nil, nil
	}
	// The edge's own table, for an edge row. The image an edge write records is
	// the LINK's columns, so the question "has this moved since" is asked of the
	// relationship row and never of either record the link joins — whose fields
	// the entry did not touch. The identifier is one of a closed set of literals
	// and never reaches here from a request.
	if !servesRecordType(entityType) && entityType != edgeEntityType {
		return nil, fmt.Errorf("compose: %q is not a record type this path reads", entityType)
	}
	asked, err := coupledImage(entityType, after)
	if err != nil {
		return nil, err
	}
	if len(asked) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT k.key
		FROM jsonb_each($2::jsonb) AS k(key, value)
		JOIN `+pgx.Identifier{entityType}.Sanitize()+` r ON r.id = $1
		WHERE to_jsonb(r) ? k.key
		  AND to_jsonb(r) -> k.key IS DISTINCT FROM k.value
		ORDER BY 1`, id, asked)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var moved []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		moved = append(moved, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	half, err := moneyMovedUnderIt(ctx, tx, row)
	if err != nil {
		return nil, err
	}
	if half != "" {
		moved = append(moved, half)
	}
	return reportedAs(moved, imageKeys(after)), nil
}

// moneyMovedUnderIt names the money half this entry states ALONE when the other
// half has been written since. Putting the amount back under a currency that
// moved states a price that never existed — wrong by the scale difference
// values.MinorUnitExceptions() encodes, and wrong silently, because the number
// is plausible in both denominations.
//
// The trail is what answers it, not the image: an update records only the
// fields the request set, so an entry that changed the amount alone carries no
// currency to compare the record against. Asking whether a LATER row wrote the
// sibling is the difference between refusing this restore and refusing every
// amount change ever made.
func moneyMovedUnderIt(ctx context.Context, tx pgx.Tx, row AuditRow) (string, error) {
	var image map[string]json.RawMessage
	if err := json.Unmarshal(row.After, &image); err != nil {
		return "", fmt.Errorf("compose: after-image is not a JSON object: %w", err)
	}
	stated, sibling := "", ""
	for i, half := range moneyPair {
		if _, ok := image[half]; ok {
			if stated != "" {
				// Both halves stated: the ordinary comparison above already
				// judged each against the record.
				return "", nil
			}
			stated, sibling = half, moneyPair[1-i]
		}
	}
	if stated == "" {
		return "", nil
	}
	var moved bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM audit_log later
		  WHERE later.entity_type = $1 AND later.entity_id = $2
		    AND later.after ? $4
		    AND (later.occurred_at, later.id) >
		        (SELECT this.occurred_at, this.id FROM audit_log this WHERE this.id = $3))`,
		row.EntityType, row.EntityID, row.ID, sibling).Scan(&moved); err != nil {
		return "", fmt.Errorf("compose: reading whether %s moved since: %w", sibling, err)
	}
	if !moved {
		return "", nil
	}
	return stated, nil
}

// Only keys the record holds as COLUMNS are compared, and the row itself says
// which those are — `to_jsonb(r) ? key`. A field kept in its own table
// (a person's social profiles, a company's domains or relationship types) is
// absent from the row's jsonb, so comparing it would read every one of them as
// moved and refuse every restore that touched one.
//
// Derived rather than listed. A hand-kept set of "fields that are not columns"
// is a second copy of the schema, and it fails the same way each time somebody
// adds one: silently, by refusing a restore that should have worked.
//
// The gap this leaves is stated: supersession does not judge those fields, so
// two people editing a company's domains in turn will not block each other the
// way two editing its name do. Judging them means reading each relation, which
// earns its own engine when it is worth building.

// coupledImage is the after-image narrowed to the keys worth comparing: the
// derived columns are dropped, because a row that changed carries a new
// updated_at whatever else moved, and comparing it would read every entry as
// superseded. The money pair's coupling is judged by moneyMovedUnderIt, which
// needs the trail rather than the image alone.
func coupledImage(entityType string, after json.RawMessage) ([]byte, error) {
	var image map[string]json.RawMessage
	if err := json.Unmarshal(after, &image); err != nil {
		return nil, fmt.Errorf("compose: after-image is not a JSON object: %w", err)
	}
	judged := map[string]json.RawMessage{}
	for key, value := range image {
		if derivedColumns[key] || provenanceStamps[entityType][key] {
			continue
		}
		judged[key] = value
	}
	if len(judged) == 0 {
		return nil, nil
	}
	return json.Marshal(judged)
}

// imageKeys is the after-image's keys, for reporting a move under the name the
// caller asked about.
func imageKeys(after json.RawMessage) []string {
	var image map[string]json.RawMessage
	if err := json.Unmarshal(after, &image); err != nil {
		return nil
	}
	keys := make([]string, 0, len(image))
	for key := range image {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
