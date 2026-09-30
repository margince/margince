// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Live List's members join and leave on their own; the scheduled check is
// what sees it. Each test drives the real check over contacts written through
// the contacts store, and reads what it recorded as a scoped reader would.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// liveListReaderPerms reads contacts and lists, team-scoped.
func liveListReaderPerms() principal.Permissions {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}, "list": {Read: true}}
	return p
}

// liveListFixture is a workspace Live List of contacts titled "Buyer", and the
// clock its checks run on.
type liveListFixture struct {
	e     *Env
	store *collections.Store
	list  ids.ListID
	clock time.Time
}

func newLiveListFixture(t *testing.T) *liveListFixture {
	t.Helper()
	e := Setup(t)
	store := compose.NewCollectionsStore(e.Pool)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{
		Name: "Buyers", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: titleIs("Buyer"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &liveListFixture{e: e, store: store, list: list.ID, clock: time.Now().UTC()}
}

func titleIs(title string) map[string]any {
	return map[string]any{"field": "title", "op": "eq", "value": title}
}

// check runs one pass a minute after the last, so every event it records is
// ordered after anything the test did before it.
func (f *liveListFixture) check(t *testing.T) []collections.LiveCheck {
	t.Helper()
	f.clock = f.clock.Add(time.Minute)
	checks, err := compose.CheckLiveLists(context.Background(), f.e.Pool, f.e.WS, f.clock)
	if err != nil {
		t.Fatalf("check the Live Lists: %v", err)
	}
	return checks
}

// contactTitled creates a contact through the contacts store, owned by Rep1.
func (f *liveListFixture) contactTitled(t *testing.T, name, title string) ids.UUID {
	t.Helper()
	created, err := f.e.Contacts.CreateContact(f.e.Admin(), contacts.CreateContactInput{
		FullName: name, Title: &title, OwnerID: userIDPtr(&f.e.Rep1), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(created.Id)
}

func (f *liveListFixture) retitle(t *testing.T, contact ids.UUID, title string) {
	t.Helper()
	if _, err := f.e.Contacts.UpdateContact(f.e.Admin(), ContactIDOf(contact),
		contacts.UpdateContactInput{Title: &title, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
}

// observed reads the entered and left entries of the list's history as ctx.
func (f *liveListFixture) observed(t *testing.T, ctx context.Context) []collections.HistoryEntry {
	t.Helper()
	history, _, err := f.store.History(ctx, f.list, 50, "")
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	var out []collections.HistoryEntry
	for _, entry := range history {
		if entry.Kind == "member_entered" || entry.Kind == "member_left" {
			out = append(out, entry)
		}
	}
	return out
}

func (f *liveListFixture) rep1() context.Context {
	return f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, liveListReaderPerms())
}

func (f *liveListFixture) outsider() context.Context {
	return f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, liveListReaderPerms())
}

func TestALiveListRecordsWhoEnteredAndWhoLeftAtTheCheckThatSawIt(t *testing.T) {
	f := newLiveListFixture(t)
	f.check(t)
	contact := f.contactTitled(t, "Bea Buyer", "Buyer")
	if got := f.observed(t, f.e.Admin()); len(got) != 0 {
		t.Fatalf("a change was recorded before any check saw it: %+v", got)
	}

	f.check(t)
	entered := f.clock
	got := f.observed(t, f.e.Admin())
	if len(got) != 1 || got[0].Kind != "member_entered" || *got[0].EntityID != contact ||
		!got[0].OccurredAt.Equal(entered) || *got[0].Reason != "evaluated" {
		t.Fatalf("after the check, history = %+v, want the contact entering at %s", got, entered)
	}

	f.retitle(t, contact, "Seller")
	f.check(t)
	got = f.observed(t, f.e.Admin())
	if len(got) != 2 || got[0].Kind != "member_left" || *got[0].EntityID != contact || !got[0].OccurredAt.Equal(f.clock) {
		t.Fatalf("after the retitle, history = %+v, want the contact leaving first", got)
	}
}

func TestTheFirstCheckTakesTheMembersWithoutRecordingThemAsJoining(t *testing.T) {
	f := newLiveListFixture(t)
	f.contactTitled(t, "Early Buyer", "Buyer")
	checks := f.check(t)
	if len(checks) != 1 || checks[0].Outcome != collections.CheckComplete || checks[0].Members != 1 || checks[0].Entered != 0 {
		t.Fatalf("the first check = %+v, want one member held and nothing recorded", checks)
	}
	if got := f.observed(t, f.e.Admin()); len(got) != 0 {
		t.Fatalf("the first check recorded %+v", got)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_live_member WHERE list_id = $1`, f.list); n != 1 {
		t.Fatalf("the first check held %d members, want 1", n)
	}
}

func TestAReaderWhoCannotSeeTheRecordSeesNeitherItsChangeNorItsCount(t *testing.T) {
	f := newLiveListFixture(t)
	f.check(t)
	for _, ctx := range []context.Context{f.rep1(), f.outsider()} {
		if _, err := f.store.VisitList(ctx, f.list); err != nil {
			t.Fatalf("visit: %v", err)
		}
	}
	// Capture-private to Rep1: the one boundary a contact read keeps.
	private := f.contactTitled(t, "Private Buyer", "Buyer")
	f.e.MakeCapturePrivate(t, "contact", private, f.e.Rep1)
	f.check(t)

	if got := f.observed(t, f.rep1()); len(got) != 1 {
		t.Fatalf("the capturer read %+v, want the entry", got)
	}
	if got := f.observed(t, f.outsider()); len(got) != 0 {
		t.Fatalf("a reader outside the contact's scope read %+v", got)
	}
	seen, err := f.store.ListView(f.rep1(), f.list)
	if err != nil || seen.SinceLastVisit == nil || seen.SinceLastVisit.Entered != 1 {
		t.Fatalf("the capturer's pulse = %+v (%v), want one joined", seen.SinceLastVisit, err)
	}
	hidden, err := f.store.ListView(f.outsider(), f.list)
	if err != nil || hidden.SinceLastVisit == nil || hidden.SinceLastVisit.Entered != 0 {
		t.Fatalf("the outsider's pulse = %+v (%v), want nothing joined", hidden.SinceLastVisit, err)
	}
	library, err := f.store.ListsPage(f.outsider(), collections.ListFilter{})
	if err != nil || len(library.Data) != 1 || library.Data[0].SinceLastVisit == nil || library.Data[0].SinceLastVisit.Entered != 0 {
		t.Fatalf("the outsider's library row = %+v (%v), want nothing joined", library.Data, err)
	}
}

func TestAChangedFilterIsComparedAgainstTheLastCheckUnderItsNewVersion(t *testing.T) {
	f := newLiveListFixture(t)
	buyer := f.contactTitled(t, "Old Match", "Buyer")
	seller := f.contactTitled(t, "New Match", "Seller")
	f.check(t)
	before, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	after, err := f.store.UpdateList(f.e.Admin(), f.list, collections.UpdateListInput{
		Definition: titleIs("Seller"), IfVersion: &before.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.check(t)
	got := f.observed(t, f.e.Admin())
	if len(got) != 2 {
		t.Fatalf("after the filter changed, history = %+v, want one entered and one left", got)
	}
	for _, entry := range got {
		if *entry.Reason != "filter_changed" || entry.DefinitionVersion == nil || *entry.DefinitionVersion != after.Version {
			t.Fatalf("entry %+v is not marked as the new filter's at version %d", entry, after.Version)
		}
		want := map[string]ids.UUID{"member_entered": seller, "member_left": buyer}[entry.Kind]
		if *entry.EntityID != want {
			t.Fatalf("%s names %s, want %s", entry.Kind, *entry.EntityID, want)
		}
	}
	f.retitle(t, buyer, "Seller")
	f.check(t)
	if latest := f.observed(t, f.e.Admin())[0]; *latest.Reason != "evaluated" {
		t.Fatalf("the check after the rebaseline still says %q", *latest.Reason)
	}
}

func TestACheckResumesWithTheListsNeverOrLongestUnchecked(t *testing.T) {
	f := newLiveListFixture(t)
	f.check(t)
	later, err := f.store.CreateList(f.e.Admin(), collections.CreateListInput{
		Name: "Sellers", EntityType: "contact", ListType: "dynamic", Sharing: "workspace", Definition: titleIs("Seller"),
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := f.check(t)
	if len(checks) != 2 || checks[0].ListID != later.ID || checks[1].ListID != f.list {
		t.Fatalf("the pass checked %+v, want the never-checked list first", checks)
	}
	view, err := f.store.ListView(f.e.Admin(), f.list)
	if err != nil || view.LastCheck == nil || !view.LastCheck.CheckedAt.Equal(f.clock) {
		t.Fatalf("the list read's last check = %+v (%v), want %s", view.LastCheck, err, f.clock)
	}
}

func TestSinceLastVisitCountsFromTheVisitBefore(t *testing.T) {
	f := newLiveListFixture(t)
	f.check(t)
	reader := f.rep1()
	first, err := f.store.VisitList(reader, f.list)
	if err != nil || first.Previous != nil {
		t.Fatalf("a first visit = %+v (%v), want no previous visit", first, err)
	}
	if view, err := f.store.ListView(reader, f.list); err != nil || view.SinceLastVisit == nil {
		t.Fatalf("after one visit the pulse = %+v (%v), want counts from it", view.SinceLastVisit, err)
	}
	f.clock = first.VisitedAt
	stays := f.contactTitled(t, "Stays", "Buyer")
	f.contactTitled(t, "Joins", "Buyer")
	f.check(t)
	f.retitle(t, stays, "Seller")
	f.check(t)
	view, err := f.store.ListView(reader, f.list)
	if err != nil || view.SinceLastVisit == nil || view.SinceLastVisit.Entered != 2 || view.SinceLastVisit.Left != 1 {
		t.Fatalf("the pulse = %+v (%v), want two joined and one left", view.SinceLastVisit, err)
	}
	second, err := f.store.VisitList(reader, f.list)
	if err != nil || second.Previous == nil || !second.Previous.Equal(first.VisitedAt) {
		t.Fatalf("the second visit = %+v (%v), want the first as its previous", second, err)
	}
}

func TestAnAgentReadingThroughAPassportIsNotAVisit(t *testing.T) {
	f := newLiveListFixture(t)
	agent := principal.WithActor(principal.WithWorkspaceID(context.Background(), f.e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: f.e.Rep1, TeamIDs: []ids.UUID{f.e.Team1},
		Permissions: liveListReaderPerms(),
	})
	if _, err := f.store.VisitList(agent, f.list); err == nil {
		t.Fatal("an agent recorded a visit for the human behind it")
	}
}

func TestErasureTakesTheSubjectOutOfTheLastCheck(t *testing.T) {
	f := newLiveListFixture(t)
	subject := f.contactTitled(t, "Erasable Buyer", "Buyer")
	f.check(t)
	pkg, err := privacy.AssembleSAR(f.e.Admin(), f.e.DB(), ids.From[ids.ContactKind](subject))
	if err != nil || len(pkg.LiveListMemberships) != 1 {
		t.Fatalf("the access export's Live Lists = %+v (%v), want the one", pkg.LiveListMemberships, err)
	}
	if err := privacy.NewEraser(f.e.DB()).EraseContact(f.e.Admin(), subject, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_live_member WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("the subject outlived the erasure in %d Live List checks", n)
	}
	f.check(t)
	if n := f.e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("the check after the erasure recorded %d events about the subject", n)
	}
}
