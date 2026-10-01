// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a list's member table and a record page read about lists, held against
// Postgres: a Live List member's filter values are hidden exactly where its why
// hides them, a Shortlist member names who chose it, and a record page names
// only the lists its reader may find, for a record they may see.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// onlyMember reads the one member a list answers for record, failing unless
// exactly that member comes back.
func onlyMember(ctx context.Context, t *testing.T, store *collections.Store, list ids.ListID, record ids.UUID) crmcontracts.ListMember {
	t.Helper()
	page, err := store.MembersPage(ctx, list, collections.MemberRead{Only: []ids.UUID{record}})
	if err != nil {
		t.Fatalf("members read: %v", err)
	}
	if len(page.Data) != 1 || ids.UUID(page.Data[0].EntityId) != record {
		t.Fatalf("members read answered %+v, want the one member %s", page.Data, record)
	}
	return page.Data[0]
}

func TestALiveListMembersValuesAreHiddenWhereItsWhyHidesThem(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	input, deal, company := dealWithHiddenCompany(t, e)
	// A reader who may not see the company is no member through its clause,
	// so the name clause keeps them one whose company value is in question.
	input.Definition = map[string]any{"or": []any{
		input.Definition,
		map[string]any{"field": "name", "op": "contains", "value": "hidden account"},
	}}
	list, err := store.CreateList(e.Admin(), input)
	if err != nil {
		t.Fatal(err)
	}
	capturer := e.As(e.Rep1, []ids.UUID{e.Team1}, dealListPerms())
	seen := (*onlyMember(capturer, t, store, list.ID, deal).Values)["company_id"]
	if seen.Hidden || seen.Value == nil || *seen.Value != company.String() || seen.Label == nil || *seen.Label != "Hidden Account" {
		t.Fatalf("the capturer read the company value %+v, want %s named Hidden Account", seen, company)
	}
	why, err := store.ExplainMember(capturer, list.ID, deal)
	if err != nil {
		t.Fatal(err)
	}
	if leaf := why.Clauses.Children[0]; leaf.ValueLabel == nil || *leaf.ValueLabel != "Hidden Account" {
		t.Fatalf("the capturer's why named the company %+v, want Hidden Account", leaf)
	}
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, dealListPerms())
	if v := (*onlyMember(outsider, t, store, list.ID, deal).Values)["company_id"]; !v.Hidden || v.Value != nil || v.Label != nil {
		t.Fatalf("a reader who cannot open the company read %+v", v)
	}
	masked := dealListPerms()
	masked.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "company_id", Condition: principal.MaskOutsideWriteAuthority}}
	maskedReader := e.As(e.Rep1, []ids.UUID{e.Team1}, masked)
	if v := (*onlyMember(maskedReader, t, store, list.ID, deal).Values)["company_id"]; !v.Hidden || v.Value != nil {
		t.Fatalf("a masked value was shown: %+v", v)
	}
}

func TestAMembersReadByRecordAnswersOnlyMembers(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	theirs := e.SeedContact(t, "Owned By Rep One", &e.Rep1)
	other := e.SeedContact(t, "Owned By Rep Two", &e.Rep2)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{
		Name: "Rep one's contacts", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "owner_id", "op": "eq", "value": e.Rep1.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := store.MembersPage(e.Admin(), list.ID, collections.MemberRead{Only: []ids.UUID{other, theirs}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 1 || ids.UUID(page.Data[0].EntityId) != theirs {
		t.Fatalf("a members read by record answered %+v, want only %s", page.Data, theirs)
	}
	_, err = store.MembersPage(e.Admin(), list.ID, collections.MemberRead{Only: []ids.UUID{theirs}, Cursor: theirs.String()})
	var bad *collections.BadInputError
	if !errors.As(err, &bad) {
		t.Fatalf("a read by record with a cursor answered %v, want a refusal", err)
	}
}

func TestAShortlistMemberNamesWhoChoseItAndWhy(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Chosen Contact", &e.Rep1)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Picks", EntityType: "contact", Sharing: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	note := "met at the fair"
	if _, err := store.AddMember(e.Admin(), list.ID, collections.MemberChange{
		EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen, Note: &note,
	}); err != nil {
		t.Fatal(err)
	}
	member := onlyMember(e.Admin(), t, store, list.ID, contact)
	if member.AddedByName == nil || *member.AddedByName == "" || member.CreatedAt == nil {
		t.Fatalf("a Shortlist member named nobody: %+v", member)
	}
	if member.Note == nil || *member.Note != note || member.Values != nil {
		t.Fatalf("a Shortlist member read %+v, want the note and no filter values", member)
	}
}

// listFinderPerms reads contacts and lists, and nothing else.
func listFinderPerms() principal.Permissions {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{"contact": {Read: true}, "list": {Read: true}}
	return p
}

func TestARecordNamesOnlyTheListsItsReaderMayFind(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Listed Contact", &e.Rep3)
	shortlist := seedList(t, e, store, collections.CreateListInput{Name: "A shared pick", EntityType: "contact", Sharing: "workspace"})
	private := seedList(t, e, store, collections.CreateListInput{Name: "A private pick", EntityType: "contact", Sharing: "private"})
	for _, list := range []ids.ListID{shortlist, private} {
		if _, err := store.AddMember(e.Admin(), list, collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen}); err != nil {
			t.Fatal(err)
		}
	}
	live := seedList(t, e, store, collections.CreateListInput{
		Name: "Rep three's", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "owner_id", "op": "eq", "value": e.Rep3.String()},
	})
	seedList(t, e, store, collections.CreateListInput{
		Name: "Rep one's", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "owner_id", "op": "eq", "value": e.Rep1.String()},
	})
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, listFinderPerms())
	lists, err := store.RecordListsFor(reader, "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	var named []ids.UUID
	for _, l := range lists.Lists {
		named = append(named, ids.UUID(l.Id))
	}
	want := []ids.UUID{shortlist.UUID, live.UUID}
	if len(named) != len(want) || named[0] != want[0] || named[1] != want[1] {
		t.Fatalf("the record named lists %v, want the shared Shortlist then the selecting Live List %v", named, want)
	}
}

func TestARecordItsReaderCannotSeeNamesNoLists(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Captured Privately", &e.Rep1)
	e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, listFinderPerms())
	if _, err := store.RecordListsFor(reader, "contact", contact); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("lists of a record the reader cannot see answered %v, want not found", err)
	}
}

func TestARecordThatDoesNotExistNamesNoLists(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	for _, entityType := range []string{"contact", "company", "deal", "lead"} {
		if _, err := store.RecordListsFor(e.Admin(), entityType, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("lists of a %s that does not exist answered %v, want not found", entityType, err)
		}
	}
}

func TestEveryLiveListIsJudgedHoweverManyAWorkspaceHas(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Listed Contact", &e.Rep3)
	// More Live Lists than one answer carries, each sorting before the one
	// that selects the record and none selecting it. Seeded in one statement:
	// a thousand writes through the list writer would be the whole test.
	e.WsExec(t, `INSERT INTO list (name, entity_type, list_type, definition, sharing, owner_id, steward_id)
		SELECT 'aa ' || lpad(g::text, 4, '0'), 'contact', 'dynamic',
		       jsonb_build_object('field', 'owner_id', 'op', 'eq', 'value', $1::text), 'workspace', $2, $2
		FROM generate_series(1, 1000) g`, e.Rep1.String(), e.AdminUser)
	selecting := seedList(t, e, store, collections.CreateListInput{
		Name: "zz Rep three's", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "owner_id", "op": "eq", "value": e.Rep3.String()},
	})
	found, err := store.RecordListsFor(e.Admin(), "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Lists) != 1 || ids.UUID(found.Lists[0].Id) != selecting.UUID || found.Truncated {
		t.Fatalf("the record named %d lists (truncated %v), want only the selecting Live List %s",
			len(found.Lists), found.Truncated, selecting)
	}
}

func TestARecordOnMoreListsThanOneAnswerCarriesSaysSo(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	contact := e.SeedContact(t, "Much Chosen", &e.Rep3)
	e.WsExec(t, `WITH made AS (
		INSERT INTO list (name, entity_type, list_type, sharing, owner_id, steward_id)
		SELECT 'pick ' || lpad(g::text, 4, '0'), 'contact', 'static', 'workspace', $2, $2
		FROM generate_series(1, 1001) g RETURNING id)
		INSERT INTO list_member (list_id, entity_type, entity_id, added_by)
		SELECT id, 'contact', $1, 'human:' || $2::text FROM made`, contact, e.AdminUser)
	found, err := store.RecordListsFor(e.Admin(), "contact", contact)
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Lists) != 1000 || !found.Truncated {
		t.Fatalf("a record on 1001 Shortlists named %d (truncated %v), want 1000 and truncated", len(found.Lists), found.Truncated)
	}
}

func seedList(t *testing.T, e *Env, store *collections.Store, input collections.CreateListInput) ids.ListID {
	t.Helper()
	list, err := store.CreateList(e.Admin(), input)
	if err != nil {
		t.Fatal(err)
	}
	return list.ID
}
