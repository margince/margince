// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The list library reads a page of lists with its counts and health in a fixed
// number of statements, however many lists the page holds.

import (
	"errors"
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listsPageFixture holds workspace Shortlists and Live Lists of contacts: each
// Shortlist holds one Buyer, and each Live List selects the Buyers.
type listsPageFixture struct {
	*liveListFixture
	created int
}

func newListsPageFixture(t *testing.T) *listsPageFixture {
	t.Helper()
	f := &listsPageFixture{liveListFixture: newLiveListFixture(t)}
	f.contactTitled(t, "Bea Buyer", "Buyer")
	f.contactTitled(t, "Ben Buyer", "Buyer")
	return f
}

// addLists creates n more Shortlists, each with one member, and n more Live Lists.
func (f *listsPageFixture) addLists(t *testing.T, n int) {
	t.Helper()
	member := f.contactTitled(t, "Chosen Buyer", "Buyer")
	for range n {
		f.created++
		short, err := f.store.CreateList(f.e.Admin(), collections.CreateListInput{
			Name: fmt.Sprintf("Short %03d", f.created), EntityType: "contact", Sharing: "workspace",
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.AddMember(f.e.Admin(), short.ID, collections.MemberChange{EntityType: "contact", EntityID: member, Reason: collections.ReasonChosen}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.CreateList(f.e.Admin(), collections.CreateListInput{
			Name: fmt.Sprintf("Live %03d", f.created), EntityType: "contact", ListType: "dynamic",
			Sharing: "workspace", Definition: titleIs("Buyer"),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// readPage reads the library as Rep1 and answers it with its statement count.
func (f *listsPageFixture) readPage(t *testing.T) ([]crmcontracts.List, int64) {
	t.Helper()
	metered, meter := database.WithStatementMeter(f.rep1())
	page, err := f.store.ListsPage(metered, collections.ListFilter{})
	if err != nil {
		t.Fatalf("read the list library: %v", err)
	}
	return page.Data, meter.Cost().Statements
}

func TestTheListLibraryReadsAnyNumberOfListsInTheSameStatements(t *testing.T) {
	t.Parallel()
	f := newListsPageFixture(t)
	f.addLists(t, 2)
	few, fewStatements := f.readPage(t)
	f.addLists(t, 18)
	many, manyStatements := f.readPage(t)

	if len(few) != 5 || len(many) != 41 {
		t.Fatalf("the library holds %d then %d lists, want 5 then 41", len(few), len(many))
	}
	t.Logf("%d lists in %d statements, %d lists in %d", len(few), fewStatements, len(many), manyStatements)
	if manyStatements != fewStatements {
		t.Fatalf("the library read %d lists in %d statements and %d lists in %d; want the same count",
			len(few), fewStatements, len(many), manyStatements)
	}
	buyers := 2 + 2
	for _, l := range many {
		want := buyers
		if l.ListType == crmcontracts.ListListTypeStatic {
			want = 1
		}
		if l.Health != crmcontracts.ListHealthOk || l.VisibleCount == nil || *l.VisibleCount != want {
			t.Fatalf("list %q: health %s, count %v; want ok and %d", l.Name, l.Health, l.VisibleCount, want)
		}
	}
}

func TestALiveListThatNoLongerCompilesLeavesTheOthersCounted(t *testing.T) {
	t.Parallel()
	f := newListsPageFixture(t)
	f.addLists(t, 2)
	f.e.WsExec(t, `UPDATE list SET definition = '{"field": "no_such_field", "op": "eq", "value": "x"}' WHERE id = $1`, f.list)

	lists, _ := f.readPage(t)
	for _, l := range lists {
		if ids.UUID(l.Id) == f.list.UUID {
			if l.Health != crmcontracts.ListHealthInvalid || l.VisibleCount != nil {
				t.Fatalf("the broken list reports health %s and count %v, want invalid and none", l.Health, l.VisibleCount)
			}
			continue
		}
		if l.Health != crmcontracts.ListHealthOk || l.VisibleCount == nil || *l.VisibleCount == 0 {
			t.Fatalf("list %q beside the broken one: health %s, count %v; want ok and counted", l.Name, l.Health, l.VisibleCount)
		}
	}
}

func TestABrokenLiveListIsRefusedToAReaderWhoMayNotReadItsRecords(t *testing.T) {
	t.Parallel()
	f := newListsPageFixture(t)
	f.e.WsExec(t, `UPDATE list SET definition = '{"field": "no_such_field", "op": "eq", "value": "x"}' WHERE id = $1`, f.list)
	perms := liveListReaderPerms()
	perms.Objects = map[string]principal.ObjectGrant{"list": {Read: true}}
	listsOnly := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, perms)

	if _, err := f.store.ListsPage(listsOnly, collections.ListFilter{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without contact read got %v from the library, want permission denied", err)
	}
	if _, err := f.store.ListView(listsOnly, f.list); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without contact read got %v from the list, want permission denied", err)
	}
}

func TestTheListLibraryUsesTheSameCountsAsOneList(t *testing.T) {
	t.Parallel()
	f := newListsPageFixture(t)
	f.addLists(t, 1)
	lists, _ := f.readPage(t)
	for _, l := range lists {
		one, err := f.store.ListView(f.rep1(), ids.From[ids.ListKind](ids.UUID(l.Id)))
		if err != nil {
			t.Fatal(err)
		}
		if one.Health != l.Health || *one.VisibleCount != *l.VisibleCount {
			t.Fatalf("list %q: the library says %s/%d, the list read %s/%d", l.Name, l.Health, *l.VisibleCount, one.Health, *one.VisibleCount)
		}
	}
}
