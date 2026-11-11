// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/worklistsnap"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// keysetPage serves rows, held newest first, from the one after the row whose
// position is after, as the suggestion stores' keysets do.
func keysetPage[T any](t *testing.T, rows []T, position func(T) (string, error), after string, limit int) ([]Positioned[T], bool) {
	t.Helper()
	positioned := make([]Positioned[T], len(rows))
	from := 0
	for i, row := range rows {
		at, err := position(row)
		if err != nil {
			t.Fatal(err)
		}
		positioned[i] = Positioned[T]{Row: row, After: at}
		if at == after {
			from = i + 1
		}
	}
	end := min(from+limit, len(rows))
	return positioned[from:end], end < len(rows)
}

type keysetDealSuggestions struct {
	t    *testing.T
	rows []crmcontracts.DealSuggestion
}

func (k keysetDealSuggestions) SuggestionsAfter(_ context.Context, _ *storekit.OwnerSet, after string, limit int) (
	[]Positioned[crmcontracts.DealSuggestion], bool, error,
) {
	page, more := keysetPage(k.t, k.rows, dealSuggestionPosition, after, limit)
	return page, more, nil
}

type keysetTagSuggestions struct {
	t    *testing.T
	rows []crmcontracts.TagSuggestion
}

func (k keysetTagSuggestions) TagSuggestionsAfter(_ context.Context, after string, limit int) (
	[]Positioned[crmcontracts.TagSuggestion], bool, error,
) {
	page, more := keysetPage(k.t, k.rows, tagSuggestionPosition, after, limit)
	return page, more, nil
}

func dealSuggestionPosition(row crmcontracts.DealSuggestion) (string, error) {
	return deals.SuggestionPositionAfter(row.CreatedAt, ids.UUID(row.Id))
}

func tagSuggestionPosition(row crmcontracts.TagSuggestion) (string, error) {
	return collections.TagSuggestionPositionAfter(row.CreatedAt, ids.UUID(row.Id))
}

// newestFirstSuggestions is count deal and count tag suggestions, each kind
// newest first, as their stores list them.
func newestFirstSuggestions(count int) ([]crmcontracts.DealSuggestion, []crmcontracts.TagSuggestion) {
	dealRows := suggestionRows(count)
	tagRows := make([]crmcontracts.TagSuggestion, count)
	for i := range count {
		dealRows[i].CreatedAt = readInstant.Add(-time.Duration(i) * time.Minute)
		tagRows[i] = tagSuggestionRow(crmcontracts.TagSuggestionEntityTypeContact)
		tagRows[i].CreatedAt = dealRows[i].CreatedAt
	}
	return dealRows, tagRows
}

// The tail resumes each suggestion kind after the last one the lane kept and
// reads both to their ends. Every suggestion is served once, above or below.
func TestTheSuggestionTailReachesEverySuggestionPastTheLane(t *testing.T) {
	const each, depth = 12, 4
	dealRows, tagRows := newestFirstSuggestions(each)
	svc := decisionsService(stubApprovals{}, stubDuplicates{}).
		WithDealSuggestions(&stubSuggestions{rows: dealRows}).WithTagSuggestions(&stubTagSuggestions{rows: tagRows}).
		WithDealSuggestionBacklog(keysetDealSuggestions{t, dealRows}).WithTagSuggestionBacklog(keysetTagSuggestions{t, tagRows})
	svc.view = viewDecisions

	lane, count, err := svc.decisionsToDepth(pageReader(), depth)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom, err := dealSuggestionPosition(dealRows[depth/2-1])
	if err != nil {
		t.Fatal(err)
	}
	if got := count.resumeAt[sourceDealSuggestion]; got != wantFrom {
		t.Fatalf("the deal suggestion tail starts at %q, want after the lane's last deal suggestion", got)
	}

	day := crmcontracts.Attention{AsOf: rankInstant}
	reading := svc.readingBacklog(pageReader(), day,
		besideDay{until: rankInstant.Add(time.Hour), zone: time.UTC, resumeAt: count.resumeAt}, false)
	bounded := map[crmcontracts.WorklistItemSource]bool{sourceDealSuggestion: true, sourceTagSuggestion: true}
	page := tailPage{scope: scopeAll, day: day, bounded: bounded, first: true}
	rest := reading.startTail(page)
	served := map[worklistsnap.Row]int{}
	above := map[worklistsnap.Row]bool{}
	for _, item := range lane {
		above[worklistsnap.Row{Source: string(item.Source), RowID: item.Id}] = true
		served[worklistsnap.Row{Source: string(item.Source), RowID: item.Id}]++
	}
	for pages := 0; rest != nil; pages++ {
		if pages > 2*each {
			t.Fatal("the suggestion tail did not end")
		}
		var rows []ranked
		var failed []crmcontracts.WorklistSourceUnavailable
		rows, rest, failed = reading.backlogTail(pageReader(), page, 5, rest, above)
		if len(failed) > 0 {
			t.Fatalf("the tail named failed sources %v", failed)
		}
		for _, row := range rows {
			served[rowIdentity(row)]++
		}
	}

	if len(served) != 2*each {
		t.Errorf("the lane and tail served %d suggestions, want all %d", len(served), 2*each)
	}
	for row, times := range served {
		if times != 1 {
			t.Errorf("%v was served %d times, want once", row, times)
		}
	}
}

// With its tail bound, a cut suggestion lane states the store's exact count:
// scrolling reaches every suggestion it counts.
func TestAContinuedSuggestionLaneStatesTheStoresTotal(t *testing.T) {
	for _, source := range []crmcontracts.AttentionItemSource{sourceDealSuggestion, sourceTagSuggestion} {
		t.Run(string(source), func(t *testing.T) {
			day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: suggestionItems(batchScanDepth, source)}
			day.Counts.ApprovalsOpen = new(0)
			open := batchScanDepth + 30
			svc := (&Service{}).WithDealSuggestionBacklog(keysetDealSuggestions{t: t}).
				WithTagSuggestionBacklog(keysetTagSuggestions{t: t})
			if source == sourceDealSuggestion {
				day.Counts.DealSuggestionsOpen = &open
			} else {
				day.Counts.TagSuggestionsOpen = &open
			}

			got := svc.worklistFrom(t.Context(), day, scopeAll, "", 25, waitingRead{}, worklistCursor{}, nil)

			count := countFor(t, got, "decisions")
			if count.Total == nil || *count.Total != open {
				t.Errorf("decisions total = %v, want the store's %d", count.Total, open)
			}
		})
	}
}

// A page that reads no tag suggestions continues none, whatever its cursor names.
func TestAReadersOwnPageContinuesNoTagSuggestions(t *testing.T) {
	_, tagRows := newestFirstSuggestions(3)
	svc := (&Service{taskScope: TasksMine}).WithTagSuggestionBacklog(keysetTagSuggestions{t, tagRows})

	batch, err := svc.moreTagSuggestions(pageReader(), tailPage{}, tailWindow{AsOf: rankInstant}, "", 5)
	if err != nil || len(batch.rows) != 0 || batch.more {
		t.Errorf("the reader's own tail read %d tag suggestions (more %v, err %v), want none", len(batch.rows), batch.more, err)
	}
}
