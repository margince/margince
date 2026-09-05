// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Dressing an admitted audit row as a line a reader can act on.

import (
	"encoding/json"
	"sort"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// linesOf orders the admitted rows and dresses as many as the page holds.
//
// ORDERED HERE, not only in SQL. Each entity type is read by its own query, so
// the arms come back individually ordered and jointly unordered; a page cut
// before this merge would take the whole of one type and none of another. The
// order is (occurred_at, id) descending — deterministic, so paging over it
// cannot repeat or skip a row when two share an instant.
func linesOf(entries []entry, limit int) []crmcontracts.MagicLine {
	sort.Slice(entries, func(a, b int) bool {
		if !entries[a].OccurredAt.Equal(entries[b].OccurredAt) {
			return entries[a].OccurredAt.After(entries[b].OccurredAt)
		}
		return entries[a].ID.String() > entries[b].ID.String()
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	out := make([]crmcontracts.MagicLine, 0, len(entries))
	for _, e := range entries {
		line, ok := lineOf(e)
		if !ok {
			continue
		}
		out = append(out, line)
	}
	return out
}

// lineOf dresses one row, or refuses it.
//
// A row whose action this build cannot explain is dropped rather than shown with
// a blank sentence: the SQL already filtered to the admitted set, so reaching
// here with an unknown action means the two lists disagree, and serving the row
// would publish that disagreement as an empty line.
func lineOf(e entry) (crmcontracts.MagicLine, bool) {
	meaning, ok := meaningOf(e.Action)
	if !ok {
		return crmcontracts.MagicLine{}, false
	}
	line := crmcontracts.MagicLine{
		Id:         openapi_types.UUID(e.ID),
		OccurredAt: e.OccurredAt,
		Lane:       crmcontracts.MagicLaneDone,
		Summary:    crmcontracts.MagicSentence{Key: meaning.sentence},
		Entity: &crmcontracts.MagicEntityRef{
			Type: e.EntityType,
			Id:   openapi_types.UUID(e.EntityID),
		},
		Actor: crmcontracts.MagicActor{
			Type: crmcontracts.MagicActorType(e.ActorType),
			Id:   e.ActorID,
		},
		Undo: &crmcontracts.MagicUndo{Undoable: false},
	}
	if meaning.consequence != "" {
		line.Consequence = &meaning.consequence
	}
	if e.OnBehalfOf != nil {
		// WHOSE authority it bound. The auto-apply sweep acts under a rep's own
		// standing decision, and a receipt saying only "agent" would hide which
		// rep had already agreed to it.
		on := openapi_types.UUID(*e.OnBehalfOf)
		line.Actor.OnBehalfOf = &on
	}
	line.Before = fieldsOf(e.Before)
	line.After = fieldsOf(e.After)
	return line, true
}

// fieldsOf lifts an audit row's before/after blob.
//
// An unreadable blob yields NO fields rather than failing the page: the line
// still says what happened and who did it, which is most of its value, and one
// stale blob must not take every other row's receipt away with it.
func fieldsOf(raw []byte) *map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return &fields
}
