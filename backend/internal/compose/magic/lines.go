// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Dressing an admitted audit row as a line a reader can act on.

import (
	"encoding/json"
	"sort"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// linesOf orders the admitted rows, dresses them, folds repeats into one line,
// and keeps as many lines as the page holds. It also answers how many rows it
// declined to show because they said nothing a reader could use.
//
// ORDERED HERE, not only in SQL. Each entity type is read by its own query, so
// the arms come back individually ordered and jointly unordered; a page cut
// before this merge would take the whole of one type and none of another. The
// order is (occurred_at, id) descending — deterministic, so paging over it
// cannot repeat or skip a row when two share an instant.
//
// FOLDED BEFORE CUT. One background job writes one audit row per record it
// touched; the page shows the job once, with a count, and cutting at the line
// limit first would have counted a hundred of twelve hundred.
func linesOf(entries []entry, limit int) (lines []crmcontracts.MagicLine, housekeeping int) {
	sort.Slice(entries, func(a, b int) bool {
		if !entries[a].OccurredAt.Equal(entries[b].OccurredAt) {
			return entries[a].OccurredAt.After(entries[b].OccurredAt)
		}
		return entries[a].ID.String() > entries[b].ID.String()
	})
	out := make([]crmcontracts.MagicLine, 0, limit)
	group := map[string]int{}
	for _, e := range entries {
		line, key, ok := lineOf(e)
		if !ok {
			housekeeping++
			continue
		}
		if at, seen := group[key]; seen {
			count := 1
			if out[at].Count != nil {
				count = *out[at].Count
			}
			count++
			out[at].Count = &count
			continue
		}
		group[key] = len(out)
		out = append(out, line)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, housekeeping
}

// lineOf dresses one row, or refuses it, and answers the key lines that say
// the same thing share.
//
// A row this build cannot describe is refused rather than shown with a blank
// or generic sentence: "A record was updated" about no named record is noise,
// and noise on this page hides the lines that matter.
func lineOf(e entry) (crmcontracts.MagicLine, string, bool) {
	d, ok := describe(e)
	if !ok {
		return crmcontracts.MagicLine{}, "", false
	}
	line := crmcontracts.MagicLine{
		Id:         openapi_types.UUID(e.ID),
		OccurredAt: e.OccurredAt,
		Lane:       crmcontracts.MagicLineLaneMagicLaneDone,
		Summary:    d.summary,
		Reason:     d.reason,
		Entity: &crmcontracts.MagicEntityRef{
			Type:  e.EntityType,
			Id:    openapi_types.UUID(e.EntityID),
			Label: e.Label,
		},
		Actor: crmcontracts.MagicActor{
			Type:  crmcontracts.MagicActorType(e.ActorType),
			Id:    e.ActorID,
			Label: ptr(actorLabel(e)),
		},
		// Undo is filled by judgeUndoOn once the page is drawn — it needs the
		// transaction and the record, neither of which this dressing has.
	}
	if meaning, admitted := meaningOf(e.Action); admitted && meaning.consequence != "" && e.Action != "update" {
		consequence := meaning.consequence
		line.Consequence = &consequence
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
	return line, groupKey(e, d), true
}

// groupKey is what two lines must share to be one line with a count: the same
// job, doing the same thing, for the same reason, to the same kind of record.
func groupKey(e entry, d description) string {
	parts := []string{e.ActorID, e.Action, e.EntityType, sentenceKey(d.summary), ""}
	if d.reason != nil {
		parts[4] = sentenceKey(*d.reason)
	}
	return strings.Join(parts, "\x00")
}

func sentenceKey(s crmcontracts.MagicSentence) string {
	if s.Values == nil {
		return s.Key
	}
	keys := make([]string, 0, len(*s.Values))
	for k := range *s.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(s.Key)
	for _, k := range keys {
		b.WriteString("|" + k + "=" + (*s.Values)[k])
	}
	return b.String()
}

func ptr[T any](v T) *T { return &v }

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

// mergeNewestFirst interleaves two newest-first line lists and keeps the page.
func mergeNewestFirst(a, b []crmcontracts.MagicLine, limit int) []crmcontracts.MagicLine {
	out := make([]crmcontracts.MagicLine, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].OccurredAt.After(out[j].OccurredAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
