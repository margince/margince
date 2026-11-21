// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The count and health of a page of lists, judged together: one transaction
// and a fixed number of statements however many lists the page holds.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// liveVocabulary is what every Live List of one record type is judged against.
type liveVocabulary struct {
	engine  storekit.Query
	retired map[string]bool
}

// liveList is a Live List whose stored filter still compiles, so it is counted.
type liveList struct {
	summary *listSummary
	pred    storekit.Predicate
}

// summarize counts what this reader may see of each list and judges its
// health. A Live List whose filter no longer compiles reports invalid and no
// count rather than failing the read, so the library still opens. A list
// nobody looks after reports that before a retired field or tag, which only
// its steward can replace.
func (s *Store) summarize(ctx context.Context, lists []listRow) ([]*listSummary, error) {
	out, err := gatedSummaries(ctx, lists)
	if err != nil {
		return nil, err
	}
	// Resolved before the transaction: the catalog reads take a connection of
	// their own (see liveFilter).
	vocab, err := s.liveVocabularies(ctx, out)
	if err != nil {
		return nil, err
	}
	shortlists, live, err := splitByKind(out, vocab)
	if err != nil {
		return nil, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		for entityType, page := range shortlists {
			if err := countShortlists(ctx, tx, entityType, page); err != nil {
				return err
			}
		}
		for entityType, page := range live {
			if err := countLiveLists(ctx, tx, vocab[entityType].engine, page); err != nil {
				return err
			}
		}
		return s.judgeHealth(ctx, tx, out, vocab)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// gatedSummaries checks read access once per record type before any filter is
// judged, so a list whose filter no longer compiles is refused to a reader who
// may not read its records.
func gatedSummaries(ctx context.Context, lists []listRow) ([]*listSummary, error) {
	out := make([]*listSummary, 0, len(lists))
	gated := map[string]bool{}
	for _, l := range lists {
		if !gated[l.EntityType] {
			if err := auth.Require(ctx, l.EntityType, principal.ActionRead); err != nil {
				return nil, err
			}
			gated[l.EntityType] = true
		}
		out = append(out, &listSummary{listRow: l, Health: healthOK, CanEdit: mayEditList(ctx, l)})
	}
	return out, nil
}

// splitByKind groups the Shortlists and the compiling Live Lists by record
// type, and marks a Live List whose filter no longer compiles invalid.
func splitByKind(summaries []*listSummary, vocab map[string]liveVocabulary) (map[string][]*listSummary, map[string][]liveList, error) {
	shortlists := map[string][]*listSummary{}
	live := map[string][]liveList{}
	for _, summary := range summaries {
		if summary.ListType != listTypeDynamic {
			shortlists[summary.EntityType] = append(shortlists[summary.EntityType], summary)
			continue
		}
		pred, err := predicateFromDefinition(summary.Definition)
		switch {
		case errors.Is(err, errNotAFilterTree):
			summary.Health = healthInvalid
		case err != nil:
			return nil, nil, fmt.Errorf("stored definition for list %s: %w", summary.ID, err)
		case !compiles(vocab[summary.EntityType].engine, pred):
			summary.Health = healthInvalid
		default:
			live[summary.EntityType] = append(live[summary.EntityType], liveList{summary: summary, pred: pred})
		}
	}
	return shortlists, live, nil
}

// liveVocabularies resolves the engine and the retired columns once per
// record type that has a Live List on the page.
func (s *Store) liveVocabularies(ctx context.Context, summaries []*listSummary) (map[string]liveVocabulary, error) {
	out := map[string]liveVocabulary{}
	for _, l := range summaries {
		if _, done := out[l.EntityType]; done || l.ListType != listTypeDynamic {
			continue
		}
		engine, ok, err := s.SegmentEngine(ctx, l.EntityType)
		if err != nil {
			return nil, err
		}
		if !ok {
			// A stored entity_type outside the segment set is an invariant break.
			return nil, fmt.Errorf("no dynamic segment engine for entity_type %q", l.EntityType)
		}
		retired, err := s.retiredColumns(ctx, l.EntityType)
		if err != nil {
			return nil, err
		}
		out[l.EntityType] = liveVocabulary{engine: engine, retired: retired}
	}
	return out, nil
}

// countShortlists counts the members this reader may see on each Shortlist of
// one record type. A list's whole size would tell them how many they cannot see.
func countShortlists(ctx context.Context, tx pgx.Tx, entityType string, page []*listSummary) error {
	byID := make(map[ids.ListID]*listSummary, len(page))
	listIDs := make([]ids.ListID, 0, len(page))
	for _, l := range page {
		byID[l.ID] = l
		listIDs = append(listIDs, l.ID)
		l.VisibleCount = new(int)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := visibleMemberClause(ctx, entityType, arg)
	if err != nil {
		return err
	}
	query := fmt.Sprintf(
		"SELECT lm.list_id, count(*) FROM list_member lm WHERE lm.list_id = ANY($%d) AND %s GROUP BY lm.list_id",
		arg(listIDs), visible)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.ListID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return err
		}
		*byID[id].VisibleCount = n
	}
	return rows.Err()
}

// countLiveLists counts what each Live List of one record type selects for
// this reader, many filters to a statement.
func countLiveLists(ctx context.Context, tx pgx.Tx, engine storekit.Query, page []liveList) error {
	for start := 0; start < len(page); start += liveBatch {
		batch := page[start:min(start+liveBatch, len(page))]
		filters := make([]storekit.Predicate, 0, len(batch))
		for _, l := range batch {
			filters = append(filters, l.pred)
		}
		counts, err := engine.CountEach(ctx, tx, filters)
		if err != nil {
			return err
		}
		for i, l := range batch {
			l.summary.VisibleCount = &counts[i]
		}
	}
	return nil
}

// judgeHealth marks each counted list ownerless, retired_field or
// retired_tag, in that order. An invalid list keeps its health.
func (s *Store) judgeHealth(ctx context.Context, tx pgx.Tx, summaries []*listSummary, vocab map[string]liveVocabulary) error {
	var counted []*listSummary
	for _, l := range summaries {
		if l.Health == healthOK {
			counted = append(counted, l)
		}
	}
	live, err := s.liveStewards(ctx, tx, counted)
	if err != nil {
		return err
	}
	named := map[*listSummary][]ids.UUID{}
	var tagIDs []ids.UUID
	for _, l := range counted {
		if l.ListType != listTypeDynamic {
			continue
		}
		pred, err := predicateFromDefinition(l.Definition)
		if err != nil {
			return err
		}
		for _, field := range storekit.FieldsNamed(pred) {
			if vocab[l.EntityType].retired[field] {
				l.RetiredFields = append(l.RetiredFields, field)
			}
		}
		named[l] = tagIDsNamed(pred, tagFieldsOf(l.EntityType))
		tagIDs = append(tagIDs, named[l]...)
	}
	archived, err := archivedTags(ctx, tx, tagIDs)
	if err != nil {
		return err
	}
	for _, l := range counted {
		for _, id := range named[l] {
			if archived[id] {
				l.RetiredTags = append(l.RetiredTags, id)
			}
		}
		slices.SortFunc(l.RetiredTags, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
		l.Health = healthOf(l, live)
	}
	return nil
}

// healthOf names the first problem of a counted list: no live steward, then a
// retired field, then a retired tag.
func healthOf(l *listSummary, live map[ids.UserID]bool) string {
	switch {
	case l.StewardID == nil || !live[*l.StewardID]:
		return healthOwnerless
	case len(l.RetiredFields) > 0:
		return healthRetiredField
	case len(l.RetiredTags) > 0:
		return healthRetiredTag
	}
	return l.Health
}

// liveStewards is which stewards of these lists may still act. Without the
// injected rule every steward counts as live.
func (s *Store) liveStewards(ctx context.Context, tx pgx.Tx, lists []*listSummary) (map[ids.UserID]bool, error) {
	out := map[ids.UserID]bool{}
	var stewards []ids.UserID
	for _, l := range lists {
		if l.StewardID != nil {
			stewards = append(stewards, *l.StewardID)
			out[*l.StewardID] = s.liveSteward == ""
		}
	}
	if s.liveSteward == "" || len(stewards) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, "SELECT u.id FROM app_user u WHERE u.id = ANY(@stewards) AND "+s.liveSteward,
		pgx.StrictNamedArgs{"stewards": stewards})
	if err != nil {
		return nil, err
	}
	live, err := pgx.CollectRows(rows, pgx.RowTo[ids.UserID])
	for _, id := range live {
		out[id] = true
	}
	return out, err
}

// archivedTags is which of these tags are archived: merged away or retired,
// so a clause on one matches no record.
func archivedTags(ctx context.Context, tx pgx.Tx, tagIDs []ids.UUID) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	if len(tagIDs) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx,
		"SELECT id FROM tag WHERE id = ANY(@"+tagIDsArg+") AND archived_at IS NOT NULL",
		pgx.StrictNamedArgs{tagIDsArg: tagIDs})
	if err != nil {
		return nil, err
	}
	archived, err := storekit.ScanUUIDColumn(rows, "archived tags the lists name")
	if err != nil {
		return nil, fmt.Errorf("read the archived tags a list names: %w", err)
	}
	for _, id := range archived {
		out[id] = true
	}
	return out, nil
}
