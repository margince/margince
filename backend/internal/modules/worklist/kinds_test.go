// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package worklist

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

func TestTheCoreCatalogLoads(t *testing.T) {
	doc, err := os.ReadFile("../../../api/worklist-kinds.yaml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(doc, events.Types())
	if err != nil {
		t.Fatal(err)
	}
	for name, kind := range catalog {
		if kind.ExtensionKey {
			t.Errorf("%s: the core catalog declares only core kinds", name)
		}
	}
}

const einvoiceKind = `
kinds:
  ext_vn.einvoice_rejected:
    category: tasks
    destination: today
    owner: [named]
    personal: false
    done_in_place: true
    delivery: push
    producer:
      subject: company
      holders: assignee
      verify: gate
      gate: content
      rebuild_on: [{event: company.updated, by: record}]
      clock: [due_at]
    max_level: agreed
`

func TestAnExtensionKindLoadsWithinItsCeiling(t *testing.T) {
	catalog, err := LoadCatalog([]byte(einvoiceKind), events.Types())
	if err != nil {
		t.Fatal(err)
	}
	if !catalog["ext_vn.einvoice_rejected"].ExtensionKey {
		t.Error("an ext_<unit>.<kind> name is an extension kind")
	}
}

func TestACatalogRefusesWhatNobodyDecided(t *testing.T) {
	cases := map[string]struct{ from, to, want string }{
		"a missing field":                          {"    done_in_place: true\n", "", "done_in_place"},
		"an unknown field":                         {"    max_level: agreed\n", "    max_level: agreed\n    score: 9\n", "score"},
		"a category outside the set":               {"category: tasks", "category: urgent", "category"},
		"an extension claiming a waiting customer": {"max_level: agreed", "max_level: waiting", "ceiling"},
		"an extension with no ceiling":             {"    max_level: agreed\n", "", "ceiling"},
		"an extension on the health screen":        {"destination: today", "destination: system_health", "today or review"},
		"a pushed kind with no producer":           {"    producer:\n", "    no_producer:\n", "no_producer"},
		"a trigger the outbox never carries":       {"company.updated", "einvoice.rejected", "not one the outbox carries"},
		"a gate-verified kind with no gate":        {"      gate: content\n", "", "names it"},
		"a holder rule outside the set":            {"holders: assignee", "holders: everyone", "producer.holders"},
		"a core name in the extension prefix":      {"ext_vn.einvoice_rejected", "ext_einvoice_rejected", "name"},
		"a name outside both shapes":               {"ext_vn.einvoice_rejected", "ext.vn.einvoice_rejected", "name"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := strings.Replace(einvoiceKind, c.from, c.to, 1)
			if doc == einvoiceKind {
				t.Fatalf("the case changed nothing: %q not found", c.from)
			}
			_, err := LoadCatalog([]byte(doc), events.Types())
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want a refusal naming %q", err, c.want)
			}
		})
	}
}
