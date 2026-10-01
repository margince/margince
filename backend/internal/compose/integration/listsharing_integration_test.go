// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A list is found by its sharing, and sharing never shows a member record the
// reader could not otherwise see: every page, count, reason and history row of
// a list is the reader's own.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listPerms is a team-scoped rep who reads contacts and works lists.
func listPerms() principal.Permissions {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		objContact: {Create: true, Read: true, Update: true},
		"list":     {Create: true, Read: true, Update: true, Delete: true},
	}
	return p
}

func listNames(reader context.Context, t *testing.T, store *collections.Store) map[string]bool {
	t.Helper()
	lists, _, err := store.ListLists(reader, collections.ListFilter{Archived: storekit.LiveOnly})
	if err != nil {
		t.Fatalf("list lists: %v", err)
	}
	out := map[string]bool{}
	for _, l := range lists {
		out[l.Name] = true
	}
	return out
}

func TestAListIsFoundByItsSharing(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, listPerms())
	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, listPerms())
	for name, sharing := range map[string]string{"Mine": "private", "Ours": "team", "Everyone's": "workspace"} {
		if _, err := store.CreateList(rep1, collections.CreateListInput{
			Name: name, EntityType: "contact", Sharing: sharing,
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	for who, want := range map[string]struct {
		reader context.Context
		sees   []string
	}{
		"its owner":           {rep1, []string{"Mine", "Ours", "Everyone's"}},
		"a teammate":          {rep2, []string{"Ours", "Everyone's"}},
		"another team's seat": {rep3, []string{"Everyone's"}},
	} {
		got := listNames(want.reader, t, store)
		if len(got) != len(want.sees) {
			t.Errorf("%s finds %v, want %v", who, got, want.sees)
		}
		for _, name := range want.sees {
			if !got[name] {
				t.Errorf("%s does not find %q: %v", who, name, got)
			}
		}
	}
}

func TestTheLibraryNarrowsToTheSharingAsked(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	for name, sharing := range map[string]string{"Mine": "private", "Ours": "team", "Everyone's": "workspace"} {
		if _, err := store.CreateList(rep1, collections.CreateListInput{
			Name: name, EntityType: "contact", Sharing: sharing,
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	for asked, want := range map[string][]string{
		"private":        {"Mine"},
		"team,workspace": {"Everyone's", "Ours"},
	} {
		lists, _, err := store.ListLists(rep1, collections.ListFilter{
			Sharing: strings.Split(asked, ","), Archived: storekit.LiveOnly,
		})
		if err != nil {
			t.Fatalf("%s: %v", asked, err)
		}
		var got []string
		for _, l := range lists {
			got = append(got, l.Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("sharing %s finds %v, want %v", asked, got, want)
		}
	}
	var bad *collections.BadInputError
	if _, _, err := store.ListLists(rep1, collections.ListFilter{Sharing: []string{"everyone"}}); !errors.As(err, &bad) {
		t.Fatalf("an unknown sharing answered %v, want a refusal naming it", err)
	}
}

func TestAListOnlyItsStewardOrAListAdminMayChange(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	rep2 := e.As(e.Rep2, []ids.UUID{e.Team1}, listPerms())
	// Shared with everyone, so the admin below can find it: list authority
	// changes a list its holder may find, and finds nothing sharing hides.
	list, err := store.CreateList(rep1, collections.CreateListInput{Name: "Targets", EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	renamed := "Rep2's now"
	_, err = store.UpdateList(rep2, list.ID, collections.UpdateListInput{Name: &renamed, IfVersion: &list.Version})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a teammate who finds the list changed it: %v, want permission denied", err)
	}
	if _, err := store.UpdateList(e.Admin(), list.ID, collections.UpdateListInput{Name: &renamed, IfVersion: &list.Version}); err != nil {
		t.Fatalf("a list admin could not change it: %v", err)
	}
}

func TestASharedListShowsEachReaderOnlyTheMembersTheyMaySee(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, listPerms())
	open := e.SeedContact(t, "Open Account", &e.Rep1)
	private := e.SeedContact(t, "Private Capture", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)

	list, err := store.CreateList(rep1, collections.CreateListInput{Name: "Dinner", EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	note := "met at the fair"
	for _, id := range []ids.UUID{open, private} {
		if _, err := store.AddMember(rep1, list.ID, collections.MemberChange{
			EntityType: "contact", EntityID: id, Note: &note, Reason: collections.ReasonChosen,
		}); err != nil {
			t.Fatalf("add %s: %v", id, err)
		}
	}

	for who, tc := range map[string]struct {
		reader context.Context
		want   int
	}{"the capturer": {rep1, 2}, "another team": {rep3, 1}} {
		count, err := store.CountMembers(tc.reader, list.ID)
		if err != nil || count != tc.want {
			t.Errorf("%s counts %d (%v), want %d", who, count, err, tc.want)
		}
		members, _, err := store.ListMembers(tc.reader, list.ID, 50, "")
		if err != nil || len(members) != tc.want {
			t.Errorf("%s pages %d members (%v), want %d", who, len(members), err, tc.want)
		}
		history, _, err := store.History(tc.reader, list.ID, 50, "")
		if err != nil {
			t.Fatal(err)
		}
		added := 0
		for _, h := range history {
			if h.Kind == "member_added" {
				added++
			}
		}
		if added != tc.want {
			t.Errorf("%s reads %d membership changes in the history, want %d", who, added, tc.want)
		}
	}
	if _, err := store.ExplainMember(rep3, list.ID, private); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("another team asked why a hidden record is on the list and was answered %v, want not found", err)
	}
	why, err := store.ExplainMember(rep1, list.ID, private)
	if err != nil || !why.Member || why.Note == nil || *why.Note != note || why.AddedBy == nil {
		t.Fatalf("the capturer's why = %+v, %v, want the member with who added it and the note", why, err)
	}
}

func TestALiveListExportHoldsOnlyTheRowsItsReaderMaySee(t *testing.T) {
	e := Setup(t)
	store := compose.NewCollectionsStore(e.Pool)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, listPerms())
	e.SeedContact(t, "Open Account", &e.Rep1)
	private := e.SeedContact(t, "Private Capture", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	list, err := store.CreateList(rep1, collections.CreateListInput{
		Name: "Rep1's book", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "owner_id", "op": "eq", "value": e.Rep1.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	for who, tc := range map[string]struct {
		reader context.Context
		want   int
	}{"the capturer": {rep1, 2}, "another team": {rep3, 1}} {
		src, err := store.ListFilterSource(tc.reader, list.ID)
		if err != nil {
			t.Fatal(err)
		}
		engine, _, err := store.SegmentEngine(tc.reader, src.Resource)
		if err != nil {
			t.Fatal(err)
		}
		result, err := compose.NewFilteredExportWriter(e.Pool).WriteListExport(tc.reader, engine, src, "json", list.ID)
		if err != nil || result.RowCount != tc.want {
			t.Errorf("%s exported %d rows (%v), want %d", who, result.RowCount, err, tc.want)
		}
		if count, err := store.CountMembers(tc.reader, list.ID); err != nil || count != tc.want {
			t.Errorf("%s counts %d members (%v), want %d", who, count, err, tc.want)
		}
	}
	if _, err := store.ExplainMember(rep3, list.ID, private); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("another team asked why a hidden record matches and was answered %v, want not found", err)
	}
	deps, err := store.Dependencies(rep1, list.ID)
	if err != nil || len(deps) != 2 || deps[0].Kind != "export" {
		t.Fatalf("the list names %+v (%v) as its uses, want the two exports", deps, err)
	}
}

// Sharing is not row scope: a seat that reads every row of every table still
// finds no colleague's private list, nor its members or history.
func TestAnAllRowsSeatDoesNotFindAColleaguesPrivateList(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	allRows := listPerms()
	allRows.RowScope = principal.RowScopeAll
	manager := e.As(e.Rep3, []ids.UUID{e.Team2}, allRows)
	private, err := store.CreateList(rep1, collections.CreateListInput{Name: "Mine alone", EntityType: "contact", Sharing: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if got := listNames(manager, t, store); got["Mine alone"] {
		t.Fatalf("an all-rows seat found a colleague's private list: %v", got)
	}
	if _, err := store.GetList(manager, private.ID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an all-rows seat read a colleague's private list: %v", err)
	}
	if _, _, err := store.History(manager, private.ID, 10, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("an all-rows seat read a colleague's private list history: %v", err)
	}
}
