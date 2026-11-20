// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchplan

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A rep's own matches are read under the rep's row scope and every narrowing.
// Each case seeds enough matches the rep must not see to fill the page alone.
// An own read that skipped a bound would answer with them.
func TestAGroupedPageReadsOwnMatchesUnderTheCallersBounds(t *testing.T) {
	e := integration.Setup(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	mine := []ids.ContactID{seedOwned(t, e, "Corwin A", e.Rep1), seedOwned(t, e, "Corwin B", e.Rep1)}
	var hidden []ids.ContactID
	for i := range 6 {
		private := seedOwned(t, e, fmt.Sprintf("Cosmo %c", 'A'+i), e.Rep3)
		owner := "owner"
		if _, err := e.Contacts.UpdateContact(e.Admin(), private, contacts.UpdateContactInput{Visibility: &owner}); err != nil {
			t.Fatal(err)
		}
		archived := seedOwned(t, e, fmt.Sprintf("Colt %c", 'A'+i), e.Rep1)
		if _, err := e.Contacts.ArchiveContact(e.Admin(), archived, nil); err != nil {
			t.Fatal(err)
		}
		hidden = append(hidden, private, archived)
	}
	store := search.NewStore(e.DB())
	page := groupedPage(rep, t, store, "co", 3, nil)
	for _, id := range hidden {
		if slices.Contains(page.ids, id.UUID) {
			t.Errorf("the rep's page holds %s, which is private to another rep or archived", id)
		}
	}
	for _, id := range mine {
		if !slices.Contains(page.ids, id.UUID) {
			t.Errorf("the rep's page lacks their own contact %s; got %v", id, page.ids)
		}
	}

	excluded := groupedPage(rep, t, store, "co", 1, func(in *search.Input) { in.Exclude = []ids.UUID{mine[0].UUID} })
	if !slices.Equal(excluded.ids, []ids.UUID{mine[1].UUID}) || excluded.more {
		t.Errorf("excluding %s left %v (more = %v), want only %s and no more", mine[0], excluded.ids, excluded.more, mine[1])
	}
	within := groupedPage(rep, t, store, "co", 1, func(in *search.Input) { in.Within = []ids.UUID{mine[0].UUID} })
	if !slices.Equal(within.ids, []ids.UUID{mine[0].UUID}) || within.more {
		t.Errorf("searching within %s left %v (more = %v), want only it and no more", mine[0], within.ids, within.more)
	}
}

// With per_type name matches and no more, the page is those and the type holds
// more, because a contained address adds one past them. One name match fewer,
// and that address is on the page.
func TestAGroupedPageAtTheCapStillCountsTheContainedAddress(t *testing.T) {
	e := integration.Setup(t)
	byName := []ids.ContactID{seedContact(t, e, "Coby A", "a@north.example"), seedContact(t, e, "Coby B", "b@north.example")}
	byAddress := seedContact(t, e, "Yuri Lenz", "yuri@cobyte.example")
	store := search.NewStore(e.DB())
	full := groupedPage(e.Admin(), t, store, "coby", 2, nil)
	if !slices.Equal(full.ids, []ids.UUID{byName[0].UUID, byName[1].UUID}) || !full.more {
		t.Errorf("per_type 2 gave %v (more = %v), want the two name matches and more", full.ids, full.more)
	}
	short := groupedPage(e.Admin(), t, store, "coby", 3, nil)
	if !slices.Contains(short.ids, byAddress.UUID) || short.more {
		t.Errorf("per_type 3 gave %v (more = %v), want the address match too and no more", short.ids, short.more)
	}
}

// Equal scores at the cap are cut by id, as the ranked search cuts them. Names
// run against creation order, so a cut by name keeps other contacts.
func TestAGroupedPageCutsEqualScoresByID(t *testing.T) {
	e := integration.Setup(t)
	var made []ids.UUID
	for i := range 6 {
		made = append(made, seedContact(t, e, fmt.Sprintf("Tib %c", 'Z'-i), fmt.Sprintf("tib%d@north.example", i)).UUID)
	}
	store := search.NewStore(e.DB())
	ranked, err := store.Search(e.Admin(), paletteInput("tib", nil, []string{"contact"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(ranked.Hits) != len(made) || ranked.Hits[0].Score != ranked.Hits[len(made)-1].Score {
		t.Fatalf("ranked %d hits with scores %v, want %d at one score", len(ranked.Hits), hitsOf(ranked.Hits, "contact"), len(made))
	}
	for i := 1; i < len(ranked.Hits); i++ {
		if ranked.Hits[i-1].Title <= ranked.Hits[i].Title {
			t.Fatalf("names do not run against ids at %d: %q then %q", i, ranked.Hits[i-1].Title, ranked.Hits[i].Title)
		}
	}
	slices.SortFunc(made, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	if page := groupedPage(e.Admin(), t, store, "tib", 3, nil); !slices.Equal(page.ids, made[:3]) {
		t.Errorf("per_type 3 kept %v, want the three lowest ids %v", page.ids, made[:3])
	}
}

type contactPage struct {
	ids  []ids.UUID
	more bool
}

// groupedPage is the contacts of one grouped search, in page order.
func groupedPage(ctx context.Context, t *testing.T, store *search.Store, query string, perType int, narrow func(*search.Input)) contactPage {
	t.Helper()
	in := paletteInput(query, &perType, nil)
	if narrow != nil {
		narrow(&in)
	}
	page, err := store.Search(ctx, in)
	if err != nil {
		t.Fatalf("grouped %q: %v", query, err)
	}
	out := contactPage{more: slices.Contains(page.TypesWithMore, "contact")}
	for _, hit := range page.Hits {
		if hit.Type == "contact" {
			out.ids = append(out.ids, hit.ID)
		}
	}
	return out
}

func seedOwned(t *testing.T, e *integration.Env, name string, owner ids.UUID) ids.ContactID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: name, OwnerID: &user, Source: provenance.RecordSourceManual})
	if err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(created.Id))
}
