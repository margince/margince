// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"slices"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stubContactCases answers the way the store does: under mine, only the
// cases with a member the recipient owns; under unassigned, only the ones
// nobody owns.
type stubContactCases struct {
	cases []ContactCase
	asked []CaseQuery
}

func (s *stubContactCases) inScope(q CaseQuery) []ContactCase {
	var out []ContactCase
	for _, c := range s.cases {
		switch {
		case q.Scope == CaseScopeMine && !slices.Contains(c.Owners, q.Recipient):
		case q.Scope == CaseScopeUnassigned && len(c.Owners) > 0:
		default:
			out = append(out, c)
		}
	}
	return out
}

func (s *stubContactCases) OpenCases(_ context.Context, q CaseQuery) ([]ContactCase, error) {
	s.asked = append(s.asked, q)
	return s.inScope(q), nil
}

func (s *stubContactCases) CountCases(_ context.Context, q CaseQuery) (int, error) {
	return len(s.inScope(q)), nil
}

func aContactCase(owners ...ids.UUID) ContactCase {
	anchor := ids.NewV7()
	return ContactCase{
		Anchor: anchor, Confidence: 0.5, Owners: owners,
		Face: crmcontracts.AttentionContactCase{
			AnchorId: openapi_types.UUID(anchor), Revision: "r1",
			Members: []crmcontracts.AttentionPairSide{{Id: openapi_types.UUID(anchor), Label: "Ann"}},
			Owners:  []string{}, MergeMode: crmcontracts.AttentionContactCaseMergeModeOrdinary,
		},
	}
}

// pageOf classifies the decision lane and narrows it the way a page does.
func pageOf(t *testing.T, svc *Service, scope string, owner ids.UUID) map[string][]string {
	t.Helper()
	items, _, err := svc.decisionsToDepth(pageReader(), needsYouPage)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]ranked, 0, len(items))
	for _, item := range items {
		rows = append(rows, classifyDecision(item, rankInstant))
	}
	kept, _ := svc.narrowToScope(pageReader(), rows, scope, owner)
	out := map[string][]string{}
	for _, row := range kept {
		out[string(row.item.Source)] = append(out[string(row.item.Source)], row.item.Id)
	}
	return out
}

// A case sits in the personal page of each member owner, and nobody else's;
// unrelated unassigned pairs stay out of it.
func TestAnAddressedCaseIsOnItsOwnersPersonalPage(t *testing.T) {
	reader, colleague, stranger := readerOf(pageReader()), ids.NewV7(), ids.NewV7()
	shared := aContactCase(reader, colleague)
	theirs := aContactCase(stranger)
	nobodys := aContactCase()
	pairs := stubDuplicates{pairs: []DuplicatePair{{ID: ids.NewV7(), EntityType: "company", LeftID: ids.NewV7(), RightID: ids.NewV7()}}, open: 1}
	for name, tc := range map[string]struct {
		scope TaskScope
		word  string
		owner ids.UUID
		want  []ContactCase
	}{
		"mine":          {TasksMine, scopeMine, ids.UUID{}, []ContactCase{shared}},
		"named member":  {TasksOwnedBy, scopeMine, colleague, []ContactCase{shared}},
		"named another": {TasksOwnedBy, scopeMine, stranger, []ContactCase{theirs}},
		"unassigned":    {TasksUnassigned, scopeUnassigned, ids.UUID{}, []ContactCase{nobodys}},
		"all":           {TasksVisible, scopeAll, ids.UUID{}, []ContactCase{shared, theirs, nobodys}},
	} {
		t.Run(name, func(t *testing.T) {
			cases := &stubContactCases{cases: []ContactCase{shared, theirs, nobodys}}
			svc := decisionsService(stubApprovals{}, pairs)
			svc.contactCases = cases
			svc.taskScope, svc.taskOwner = tc.scope, tc.owner
			page := pageOf(t, svc, tc.word, tc.owner)
			var want []string
			for _, c := range tc.want {
				want = append(want, c.Anchor.String())
			}
			if got := page[sourceContactCase]; !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
				t.Errorf("cases on the page %v, want %v", got, want)
			}
			personal := tc.scope == TasksMine || tc.scope == TasksOwnedBy
			if personal && len(page[sourceDuplicate]) != 0 {
				t.Errorf("an unrelated unassigned pair reached a personal page: %v", page[sourceDuplicate])
			}
		})
	}
}

// The personal case read still runs when the pair store fails, and the pair
// store is not asked.
func TestAPersonalPagesCasesDoNotDependOnThePairStore(t *testing.T) {
	reader := readerOf(pageReader())
	unavailable := errors.New("the pair store is down")
	cases := &stubContactCases{cases: []ContactCase{aContactCase(reader)}}
	svc := decisionsService(stubApprovals{}, stubDuplicates{err: unavailable, settleErr: unavailable})
	svc.contactCases = cases
	svc.taskScope = TasksMine
	rows, count, err := svc.decisionsToDepth(pageReader(), needsYouPage)
	if err != nil || len(rows) != 1 || count.duplicates != 1 || count.items != 1 {
		t.Fatalf("rows=%d count=%+v err=%v", len(rows), count, err)
	}
	if len(cases.asked) != 1 || cases.asked[0].Recipient != reader || cases.asked[0].Scope != CaseScopeMine {
		t.Errorf("the case read was asked %+v, want mine for the reader", cases.asked)
	}
}

// A reader's case is on the first page however many stronger pairs wait.
func TestACaseIsReachedBeforeAFloodOfStrongerPairs(t *testing.T) {
	reader := readerOf(pageReader())
	flood := make([]DuplicatePair, batchScanDepth)
	for i := range flood {
		flood[i] = DuplicatePair{ID: ids.NewV7(), EntityType: "company", Confidence: 0.99, LeftID: ids.NewV7(), RightID: ids.NewV7()}
	}
	mine := aContactCase(reader)
	for _, scope := range []TaskScope{TasksMine, TasksVisible} {
		svc := decisionsService(stubApprovals{}, stubDuplicates{pairs: flood, open: len(flood)})
		svc.contactCases = &stubContactCases{cases: []ContactCase{mine}}
		svc.taskScope = scope
		rows, count, err := svc.decisionsToDepth(pageReader(), needsYouPage)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(rows, func(item crmcontracts.AttentionItem) bool { return item.Id == mine.Anchor.String() }) {
			t.Errorf("scope %d: the reader's case is not on a page of %d", scope, len(rows))
		}
		if wantPairs := map[TaskScope]int{TasksMine: 0, TasksVisible: len(flood)}[scope]; count.duplicates != wantPairs+1 {
			t.Errorf("scope %d: duplicates counted %d, want %d", scope, count.duplicates, wantPairs+1)
		}
	}
}
