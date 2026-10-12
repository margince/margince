// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The counts beside a settings catalog row are record data, so each is the
// caller's own. A count including rows the caller cannot open tells them
// those rows exist.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/installseam"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// listedTagReach reads the tag list as ctx and answers each tag's carried_by
// by name.
func listedTagReach(ctx context.Context, t *testing.T, store *collections.Store, params crmcontracts.ListTagsParams) map[string]*int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/tags", nil).WithContext(ctx)
	collections.NewHandlers(store).ListTags(rec, req, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("tag list: status %d, body %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode the tag list: %v", err)
	}
	out := make(map[string]*int, len(page.Data))
	for _, tag := range page.Data {
		out[tag.Name] = tag.CarriedBy
	}
	return out
}

// A contact still private to the rep who captured it is the row a colleague's
// count must leave out. Every other contact is read by every seat.
func TestTheTagListCountsOnlyWhatTheCallerMaySee(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	curator := tagCurator(e)
	shared := e.SeedContact(t, "Shared Contact", &e.Rep1)
	private := e.SeedContact(t, "Captured By Rep Two", &e.Rep2)
	word, err := store.CreateTag(curator, "Scoped reach", nil, nil)
	if err != nil {
		t.Fatalf("creating the tag: %v", err)
	}
	for _, contact := range []ids.UUID{shared, private} {
		if _, err := store.ApplyTag(curator, word.ID, "contact", contact); err != nil {
			t.Fatalf("applying the tag: %v", err)
		}
	}
	e.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, private, e.Rep2)
	unused, err := store.CreateTag(curator, "Carried by nothing", nil, nil)
	if err != nil {
		t.Fatalf("creating the unused tag: %v", err)
	}

	seat := func(user ids.UUID, objects map[string]principal.ObjectGrant) context.Context {
		return e.As(user, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"custom"}, Objects: objects, RowScope: principal.RowScopeOwn,
		})
	}
	readsContacts := map[string]principal.ObjectGrant{"tag": {Read: true}, "contact": {Read: true}}
	for _, c := range []struct {
		name   string
		reader context.Context
		want   int
	}{
		{"the rep the private contact belongs to", seat(e.Rep2, readsContacts), 2},
		{"a colleague", seat(e.Rep1, readsContacts), 1},
		{"a seat that may not read contacts", seat(e.Rep2, map[string]principal.ObjectGrant{"tag": {Read: true}}), 0},
	} {
		reach := listedTagReach(c.reader, t, store, crmcontracts.ListTagsParams{WithCarriedBy: new(true)})
		if got := reach[word.Name]; got == nil || *got != c.want {
			t.Errorf("%s: carried_by = %s, want %d", c.name, countText(got), c.want)
		}
		if got := reach[unused.Name]; got == nil || *got != 0 {
			t.Errorf("%s: an unused tag reads carried_by %s, want 0", c.name, countText(got))
		}
	}
}

// The count reads every tagging, so a list read that does not ask for it
// pays nothing for it.
func TestTheTagListCountsNothingUnlessAsked(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	curator := tagCurator(e)
	contact := e.SeedContact(t, "Tagged Contact", &e.Rep1)
	word, err := store.CreateTag(curator, "Uncounted", nil, nil)
	if err != nil {
		t.Fatalf("creating the tag: %v", err)
	}
	if _, err := store.ApplyTag(curator, word.ID, "contact", contact); err != nil {
		t.Fatalf("applying the tag: %v", err)
	}
	for name, asked := range map[string]*bool{"left off": nil, "false": new(false)} {
		reach := listedTagReach(e.Admin(), t, store, crmcontracts.ListTagsParams{WithCarriedBy: asked})
		got, listed := reach[word.Name]
		if !listed {
			t.Fatalf("with_carried_by %s: the tag is missing from the list", name)
		}
		if got != nil {
			t.Errorf("with_carried_by %s: carried_by = %d, want absent", name, *got)
		}
	}
}

// countText prints an optional count, telling absent from zero.
func countText(n *int) string {
	if n == nil {
		return "absent"
	}
	return strconv.Itoa(*n)
}

// listedDealCounts reads the acquisition catalog as ctx and answers each
// source's deal_count by key.
func listedDealCounts(ctx context.Context, t *testing.T, e *Env) map[string]*int {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/acquisition-sources", nil).WithContext(ctx)
	deals.NewHandlers(e.DB(), installseam.Deals()).ListAcquisitionSources(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("acquisition sources: status %d, body %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.AcquisitionSourceListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode the catalog: %v", err)
	}
	out := make(map[string]*int, len(page.Data))
	for _, source := range page.Data {
		out[source.Key] = source.DealCount
	}
	return out
}

// referredDealsListed counts the live deals carrying source that the deal list
// shows ctx in one pipeline.
func referredDealsListed(ctx context.Context, t *testing.T, e *Env, pipeline ids.PipelineID, source string) int {
	t.Helper()
	listed, _, err := e.Deals.ListDeals(ctx, deals.ListDealsInput{PipelineID: &pipeline})
	if err != nil {
		t.Fatalf("listing the pipeline's deals: %v", err)
	}
	n := 0
	for _, deal := range listed {
		if deal.AcquisitionSource != nil && *deal.AcquisitionSource == source {
			n++
		}
	}
	return n
}

// A deal is workspace-readable, so an own-scope seat reads a colleague's deals
// and counts them too. The count agrees with the deal list that seat reads.
func TestTheAcquisitionCatalogCountsTheLiveDealsTheCallerMaySee(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	pipeline, open, _ := DealFixture(t, e)
	referral := "referral"
	refer := func(name string, owner *ids.UUID) ids.DealID {
		deal := ids.From[ids.DealKind](e.SeedDeal(t, name, pipeline, open, owner))
		if _, err := e.Deals.UpdateDeal(admin, deal, deals.UpdateDealInput{AcquisitionSource: &referral}); err != nil {
			t.Fatalf("attributing %q: %v", name, err)
		}
		return deal
	}
	refer("Referred one", &e.Rep1)
	refer("Referred two", &e.Rep1)
	refer("Referred to rep two", &e.Rep2)
	if _, err := e.Deals.ArchiveDeal(admin, refer("Referred and archived", &e.Rep1), nil); err != nil {
		t.Fatalf("archiving a referred deal: %v", err)
	}

	seat := func(user ids.UUID, scope principal.RowScope, objects map[string]principal.ObjectGrant) context.Context {
		return e.As(user, []ids.UUID{e.Team1}, principal.Permissions{
			RoleKeys: []string{"custom"}, Objects: objects, RowScope: scope,
		})
	}
	readsDeals := map[string]principal.ObjectGrant{"custom_field": {Read: true}, "deal": {Read: true}}
	for _, c := range []struct {
		name   string
		reader context.Context
		want   int
	}{
		{"a seat that sees every deal", seat(e.Rep2, principal.RowScopeAll, readsDeals), 3},
		{"rep one, scoped to their own deals", seat(e.Rep1, principal.RowScopeOwn, readsDeals), 3},
		{"rep two, scoped to their own deals", seat(e.Rep2, principal.RowScopeOwn, readsDeals), 3},
	} {
		counts := listedDealCounts(c.reader, t, e)
		if got := counts[referral]; got == nil || *got != c.want {
			t.Errorf("%s: referral deal_count = %s, want %d", c.name, countText(got), c.want)
		}
		if listed := referredDealsListed(c.reader, t, e, pipeline, referral); listed != c.want {
			t.Errorf("%s: the deal list shows %d referred deals, the catalog counts %d", c.name, listed, c.want)
		}
		if got := counts["inbound"]; got == nil || *got != 0 {
			t.Errorf("%s: an unused source reads deal_count %s, want 0", c.name, countText(got))
		}
	}
	vocabularyOnly := seat(e.Rep2, principal.RowScopeAll, map[string]principal.ObjectGrant{"custom_field": {Read: true}})
	if got := listedDealCounts(vocabularyOnly, t, e)[referral]; got != nil {
		t.Errorf("a seat that may not read deals was told %d deals carry the source, want no count", *got)
	}
}
