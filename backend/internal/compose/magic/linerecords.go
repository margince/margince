// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// Opening a done line: every record it stands for, what changed on each, and
// whether that one change can be taken back.
//
// A line folds one job's work into a sentence and a count — "Changed industry,
// offer summary: GEM and 150 more". That is the right summary and the wrong
// place to stop: a reader who disagrees with the machine has to see WHICH
// records it changed, from what to what, and be able to put one back. The line
// had no way to show any of it, because grouping kept only the ids.
//
// The records are found by regrouping: the same window, the same row scope and
// the same group key that drew the line. A second assembly with its own rules
// would be a second answer to "what did this line count", and the two would
// drift.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordsPageDefault is how many records one page of an opened line holds when
// the caller does not say.
const recordsPageDefault = 50

// LineRecords answers one page of the records the line `lineID` stands for, in
// the window starting at `since`. ErrNotFound when no line in that window has
// that id — a retention line included, which names no record by design.
func (s *Service) LineRecords(
	ctx context.Context, lineID ids.UUID, since time.Time, cursor *string, limit int,
) (crmcontracts.MagicLineRecords, error) {
	from, err := s.windowStart(ctx, &since, s.now().UTC())
	if err != nil {
		return crmcontracts.MagicLineRecords{}, err
	}
	if limit <= 0 {
		limit = recordsPageDefault
	}
	limit = min(limit, maxLimit)
	pos, err := decodeRecordsCursor(cursor, s.now().UTC())
	if err != nil {
		return crmcontracts.MagicLineRecords{}, err
	}
	offset := pos.offset
	out := crmcontracts.MagicLineRecords{Data: []crmcontracts.MagicLineRecord{}}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		entries, _, _, err := doneSince(ctx, tx, from, maxLimit)
		if err != nil {
			return err
		}
		members, found := lineMembers(asOf(entries, pos.asOf), lineID)
		if !found {
			return apperrors.ErrNotFound
		}
		end := min(offset+limit, len(members))
		if offset < end {
			out.Data, err = s.recordsOf(ctx, tx, members[offset:end])
			if err != nil {
				return err
			}
		}
		if end < len(members) {
			next := recordsCursor{offset: end, asOf: pos.asOf}.encode()
			out.Page = crmcontracts.PageInfo{HasMore: true, NextCursor: &next}
		}
		total := len(members)
		out.Page.Total = &total
		return nil
	})
	if err != nil {
		return crmcontracts.MagicLineRecords{}, fmt.Errorf("open a done line: %w", err)
	}
	return out, nil
}

// lineMembers regroups the window and returns the entries that share the named
// line's group key, newest first and one per record: the same fold linesOf
// makes, so the count a line shows and the records it opens to are one answer.
func lineMembers(entries []entry, lineID ids.UUID) ([]entry, bool) {
	sort.Slice(entries, func(a, b int) bool {
		if !entries[a].OccurredAt.Equal(entries[b].OccurredAt) {
			return entries[a].OccurredAt.After(entries[b].OccurredAt)
		}
		return entries[a].ID.String() > entries[b].ID.String()
	})
	var key string
	for _, e := range entries {
		if e.ID == lineID {
			k, ok := groupKeyOf(e)
			if !ok {
				return nil, false
			}
			key = k
			break
		}
	}
	if key == "" {
		return nil, false
	}
	seen := map[ids.UUID]bool{}
	var members []entry
	for _, e := range entries {
		if seen[e.EntityID] {
			continue
		}
		if k, ok := groupKeyOf(e); ok && k == key {
			seen[e.EntityID] = true
			members = append(members, e)
		}
	}
	return members, true
}

// recordsOf dresses a page of members and asks the undo judge about each, in
// one call for the page.
func (s *Service) recordsOf(ctx context.Context, tx pgx.Tx, members []entry) ([]crmcontracts.MagicLineRecord, error) {
	out := make([]crmcontracts.MagicLineRecord, 0, len(members))
	subjects := make([]UndoSubject, 0, len(members))
	for _, e := range members {
		changes, err := visibleChanges(ctx, e.EntityType, fieldChanges(e.Before, e.After))
		if err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.MagicLineRecord{
			AuditId:    openapi_types.UUID(e.ID),
			OccurredAt: e.OccurredAt,
			Entity: crmcontracts.MagicEntityRef{
				Type: e.EntityType, Id: openapi_types.UUID(e.EntityID), Label: e.Label,
			},
			Changes: changes,
		})
		subjects = append(subjects, UndoSubject{AuditID: e.ID, EntityType: e.EntityType})
	}
	answers := map[ids.UUID]*crmcontracts.MagicUndo{}
	if s.undo != nil {
		var err error
		if answers, err = s.undo.JudgeUndoPage(ctx, tx, subjects); err != nil {
			return nil, err
		}
	}
	for i := range out {
		answer, ok := answers[ids.UUID(out[i].AuditId)]
		if !ok {
			answer = refusedUndo(unwiredUndoReason)
		}
		out[i].Undo = *answer
		if err := attachVersion(ctx, tx, &out[i].Undo, out[i].Entity.Type, ids.UUID(out[i].Entity.Id)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fieldChanges lists every field an audit image moved, old value to new, sorted
// by field. The machinery's own keys are left out, the same set describe()
// leaves out of the sentence.
func fieldChanges(beforeRaw, afterRaw []byte) []crmcontracts.MagicFieldChange {
	before, after := objectOf(beforeRaw), objectOf(afterRaw)
	out := []crmcontracts.MagicFieldChange{}
	for k, v := range after {
		if bookkeepingFields[k] || evidenceKeys[k] {
			continue
		}
		b, had := before[k]
		if had && equalJSON(b, v) {
			continue
		}
		out = append(out, crmcontracts.MagicFieldChange{Field: k, Before: b, After: v})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Field < out[b].Field })
	return out
}

// evidenceKeys are the keys an older writer put in the after-image that say
// where it read rather than what it wrote (privacy's writerBookkeepingKeys).
var evidenceKeys = map[string]bool{
	"source": true, "source_url": true, "source_ref": true,
	"fields": true, "human_fields": true, "facts": true,
	"cohort_linked": true, "cohort_promoted": true,
}

// maxRecordsOffset bounds how far into a line a cursor may point: far past any
// line readCap can draw, and small enough that offset+limit cannot overflow.
const maxRecordsOffset = 1_000_000

// recordsCursor is a page position in an opened line: how many records came
// before, and the instant the first page was read. Later pages drop anything
// newer than that instant, so a change landing between two pages cannot shift
// the list under the reader and skip or repeat a record.
type recordsCursor struct {
	offset int
	asOf   time.Time
}

func (c recordsCursor) encode() string {
	return strconv.Itoa(c.offset) + ":" + strconv.FormatInt(c.asOf.UnixNano(), 10)
}

// decodeRecordsCursor reads a position a previous page handed out; no cursor
// is the first page, read as of now.
func decodeRecordsCursor(cursor *string, now time.Time) (recordsCursor, error) {
	if cursor == nil || *cursor == "" {
		return recordsCursor{asOf: now}, nil
	}
	offsetText, asOfText, ok := strings.Cut(*cursor, ":")
	offset, errOffset := strconv.Atoi(offsetText)
	nanos, errAsOf := strconv.ParseInt(asOfText, 10, 64)
	if !ok || errOffset != nil || errAsOf != nil || offset < 0 || offset > maxRecordsOffset {
		return recordsCursor{}, fmt.Errorf("%w: cursor", apperrors.ErrInvalidArgument)
	}
	return recordsCursor{offset: offset, asOf: time.Unix(0, nanos).UTC()}, nil
}

// asOf keeps the entries that had happened by the instant a line was opened.
func asOf(entries []entry, at time.Time) []entry {
	out := entries[:0:0]
	for _, e := range entries {
		if !e.OccurredAt.After(at) {
			out = append(out, e)
		}
	}
	return out
}
