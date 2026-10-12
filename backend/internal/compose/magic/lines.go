// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Dressing an admitted audit row as a line a reader can act on.

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
func linesOf(mask imageMask, entries []entry, capped map[string]bool, limit int) (lines []crmcontracts.MagicLine, housekeeping int) {
	sort.Slice(entries, func(a, b int) bool {
		if !entries[a].OccurredAt.Equal(entries[b].OccurredAt) {
			return entries[a].OccurredAt.After(entries[b].OccurredAt)
		}
		return entries[a].ID.String() > entries[b].ID.String()
	})
	out := make([]crmcontracts.MagicLine, 0, limit)
	group := map[string]int{}
	// The RECORDS each line stands for, so two passes of one job over the same
	// contact count it once: the count reads "N records", not N audit rows.
	records := map[int]map[ids.UUID]bool{}
	for _, e := range entries {
		line, key, ok := lineOf(mask, e)
		if !ok {
			housekeeping++
			continue
		}
		if at, seen := group[key]; seen {
			records[at][e.EntityID] = true
			count := len(records[at])
			out[at].Count = &count
			switch {
			case count > 1:
				out[at].Reason = reasonForMany(out[at].Reason)
			case isSiteReason(out[at].Reason) && !sameReason(out[at].Reason, line.Reason):
				// One record, read from two sites: naming the newer one would
				// hide the other, so the line names neither.
				out[at].Reason = &crmcontracts.MagicSentence{Key: whySiteUnnamed}
			}
			continue
		}
		group[key] = len(out)
		records[len(out)] = map[ids.UUID]bool{e.EntityID: true}
		if capped[armOfEntry(e)] {
			// Its arm was cut, so this line's records were counted out of a
			// partial read: what it shows is the floor, whether it ends up
			// standing for one record or five thousand.
			floor := true
			line.CountIsFloor = &floor
		}
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
func lineOf(mask imageMask, e entry) (crmcontracts.MagicLine, string, bool) {
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
			Label: new(actorLabel(e)),
		},
		// Undo is filled by judgeUndoOn once the page is drawn — it needs the
		// transaction and the record, neither of which this dressing has.
	}
	if meaning, admitted := meaningOf(e.Action); admitted && meaning.consequence != "" && e.Action != actionUpdate {
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
	// A create's image is the whole new record and an archive's is empty;
	// neither says more than the line's sentence and the record's name.
	if !bulkActions[e.Action] {
		line.Before = mask.withheldFrom(e.EntityType, fieldsOf(e.Before))
		line.After = mask.withheldFrom(e.EntityType, fieldsOf(e.After))
	}
	return line, groupKey(e, d), true
}

// groupKeyOf answers what lineOf would group this row under, without dressing
// the line: the members read wants the key and nothing else, and a line it
// discards would need a reader's mask to be built at all.
func groupKeyOf(e entry) (string, bool) {
	d, ok := describe(e)
	if !ok {
		return "", false
	}
	return groupKey(e, d), true
}

// groupKey is what two lines must share to be one line with a count: the same
// job, doing the same thing, for the same reason, to the same kind of record.
func groupKey(e entry, d description) string {
	parts := []string{e.ActorID, e.Action, e.EntityType, sentenceKey(d.summary), "", "", ""}
	if d.reason != nil {
		parts[4] = sentenceKey(*d.reason)
		if many, perRecord := manyReasons[d.reason.Key]; perRecord {
			parts[4] = many
		}
	}
	// Whose authority it ran under: the auto-apply sweep acts for each rep on
	// their own standing decision, and folding two reps' actions into one line
	// would name only one of them.
	if e.OnBehalfOf != nil {
		parts[5] = e.OnBehalfOf.String()
	}
	// A bulk verb folds per day: "created 2,146 contacts" is one line for the
	// day it happened, and an import that ran over a week reads as seven.
	if bulkActions[e.Action] {
		parts[6] = e.OccurredAt.UTC().Format(time.DateOnly)
	}
	return strings.Join(parts, "\x00")
}

// manyReasons are the reasons whose values belong to ONE record — the website a
// company's details were read on — mapped to what a line standing for many
// records says instead. Grouped on the per-record values, a website reader run
// over 234 companies would be 234 lines again; grouped without them, one line
// would name one company's site as the source for all of them.
var manyReasons = map[string]string{
	"magic.why.site_read": "magic.why.site_read_each",
	whySiteUnnamed:        "magic.why.site_read_each",
}

// reasonForMany is the reason a line says once it stands for more than one record.
func reasonForMany(r *crmcontracts.MagicSentence) *crmcontracts.MagicSentence {
	if r == nil {
		return nil
	}
	if many, perRecord := manyReasons[r.Key]; perRecord {
		return &crmcontracts.MagicSentence{Key: many}
	}
	return r
}

// isSiteReason reports a reason naming the website one record was read on.
func isSiteReason(r *crmcontracts.MagicSentence) bool {
	if r == nil {
		return false
	}
	_, perRecord := manyReasons[r.Key]
	return perRecord
}

// sameReason reports two reasons saying the same thing, values included.
func sameReason(a, b *crmcontracts.MagicSentence) bool {
	if a == nil || b == nil {
		return a == b
	}
	return sentenceKey(*a) == sentenceKey(*b)
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
