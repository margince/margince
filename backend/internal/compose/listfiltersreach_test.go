// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// list_records' filter vocabulary is assembled from two halves that live in
// different repositories of truth — the contract's declared list parameters and
// what a module's store can actually bind — and this is the only layer that can
// see both ends of the wire the halves travel down.
//
// The gate below is about the WIRE, not about either half. Each half has its own
// check where it lives (the generator reads crm.yaml; each module proves every
// filter it declares narrows something). What neither can see is a break
// between them: a provider that stopped delegating, a record type dropped from
// the enumeration set, a registration that never happened. Every one of those
// fails the same way — quietly, with a tool that still answers and simply never
// narrows.

import (
	"github.com/margince/margince/backend/internal/modules/agents"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Every filter a store binds reaches the tool's published schema, UNDER ITS OWN
// RECORD TYPE.
//
// The per-type half is the whole point. Most of these names are shared — four
// of the five types bind `owner_id` — so a check that asked only whether the
// schema mentions a name would stay green with an entire record type dropped
// from the published vocabulary, since its siblings still mention every name it
// had. That is exactly the break this layer exists to catch.
//
// A nil pool is enough: the vocabulary is a property of the code, resolved at
// registration, and no part of answering it touches the database.
func TestEveryFilterAStoreBindsReachesTheAgentSurface(t *testing.T) {
	published := agents.ListFiltersDocument(NewProvider(nil))
	for _, recordType := range []datasource.EntityType{
		datasource.EntityContact, datasource.EntityCompany, datasource.EntityDeal, datasource.EntityLead,
		datasource.EntityProject, datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityActivity,
	} {
		bound := NewProvider(nil).ListFilters(recordType)
		if len(bound) == 0 {
			t.Errorf("%s binds no filters at all, so it can only be listed whole — "+
				"if that is a wiring break rather than the truth, it is invisible from the tool",
				recordType)
			continue
		}
		var offered []string
		for _, filter := range published.RecordTypes[string(recordType)] {
			offered = append(offered, filter.Name)
		}
		for _, name := range bound {
			if !slices.Contains(offered, name) {
				t.Errorf("%s binds %q and describe_list_filters publishes %v for that type: capability nobody "+
					"can reach. A filter is published only when crm.yaml's list operation declares it "+
					"too, so the fix is usually the missing parameter there.", recordType, name, offered)
			}
		}
	}
}

// The timeline is listed by name, as GET /activities lists it, and never swept.
// search_records sweeps every type when none is named, so it does not offer it.
func TestTheTimelineIsListedButNeverSwept(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	list, listed := registry.Spec("list_records")
	search, searched := registry.Spec("search_records")
	if !listed || !searched {
		t.Fatal("list_records or search_records is not registered, so this proves nothing about what they offer")
	}
	if !strings.Contains(string(list.InputSchema), `"activity"`) {
		t.Errorf("list_records does not offer activity, so a passport cannot list the timeline it lists on REST:\n%s", list.InputSchema)
	}
	if strings.Contains(string(search.InputSchema), `"activity"`) {
		t.Errorf("search_records offers activity in its record_type enum:\n%s", search.InputSchema)
	}
}
