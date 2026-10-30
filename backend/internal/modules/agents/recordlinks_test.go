// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestEvidenceLinksTheRecordsTheAppHasAPageFor(t *testing.T) {
	deal, activity := ids.NewV7(), ids.NewV7()
	refs := []EvidenceRef{
		{RecordType: datasource.EntityDeal, RecordID: deal},
		{RecordType: datasource.EntityActivity, RecordID: activity},
	}
	linkEvidence(refs, "https://crm.example.test")

	if want := "https://crm.example.test/#/deals/" + deal.String(); refs[0].WebURL != want {
		t.Errorf("deal link = %q, want %q", refs[0].WebURL, want)
	}
	if refs[1].WebURL != "" {
		t.Errorf("an activity has no page of its own, and was linked to %q", refs[1].WebURL)
	}

	unlinked := []EvidenceRef{{RecordType: datasource.EntityDeal, RecordID: deal}}
	linkEvidence(unlinked, "")
	if unlinked[0].WebURL != "" {
		t.Errorf("with no configured origin the deal was linked to %q", unlinked[0].WebURL)
	}
}

func TestEveryAmountBesideACurrencyGetsAReadableFigure(t *testing.T) {
	in := json.RawMessage(`{"deals":[` +
		`{"name":"Acme","amount_minor":7200000,"currency":"USD"},` +
		`{"name":"Tokyo","amount_minor":500000,"currency":"JPY"},` +
		`{"name":"Kept","amount_minor":100,"amount_display":"set by the tool","currency":"EUR"},` +
		`{"name":"Unpriced"}],` +
		`"count":9007199254740993,"weighted_minor":123456,"base_currency":"EUR"}`)

	var got struct {
		Deals    []map[string]any `json:"deals"`
		Count    json.Number      `json:"count"`
		Weighted string           `json:"weighted_display"`
	}
	decoder := json.NewDecoder(bytes.NewReader(withReadableAmounts(in)))
	decoder.UseNumber()
	if err := decoder.Decode(&got); err != nil {
		t.Fatal(err)
	}
	for i, want := range []any{"72000.00 USD", "500000 JPY", "set by the tool", nil} {
		if display := got.Deals[i]["amount_display"]; display != want {
			t.Errorf("deal %d amount_display = %v, want %v", i, display, want)
		}
	}
	if got.Weighted != "1234.56 EUR" {
		t.Errorf("a forecast figure beside its base_currency reads %q", got.Weighted)
	}
	if got.Count != "9007199254740993" {
		t.Errorf("an integer past float64 precision came back as %s", got.Count)
	}
}

func TestAResultWithNoAmountKeepsItsExactBytes(t *testing.T) {
	in := json.RawMessage(`{"z":1,  "a":"currency"}`)
	if out := withReadableAmounts(in); string(out) != string(in) {
		t.Errorf("a result with no money pair was re-rendered: %s", out)
	}
}

func TestTheServerNamesItsWebsiteAndIconOnlyWhenItKnowsItsOrigin(t *testing.T) {
	bare := promptDispatcher(t).identity()
	if bare[fieldTitle] != serverTitle {
		t.Errorf("serverInfo title = %v, want %q", bare[fieldTitle], serverTitle)
	}
	if _, has := bare["icons"]; has {
		t.Error("with no configured origin serverInfo still names icons it has no address for")
	}

	served := promptDispatcher(t)
	WithPublicOrigin("https://crm.example.test/")(served)
	info := served.identity()
	if info["websiteUrl"] != "https://crm.example.test" {
		t.Errorf("websiteUrl = %v", info["websiteUrl"])
	}
	icons, ok := info["icons"].([]map[string]any)
	if !ok || len(icons) == 0 || icons[0]["src"] != "https://crm.example.test/icon.svg" {
		t.Errorf("icons = %v, want the app's own icon on its origin", info["icons"])
	}
}

// whoami answers about the caller and reads no rows. A row-scope warning on it
// would only teach a model to skim past the warning everywhere else.
func TestAnAnswerAboutTheCallerCarriesNoRowScopeWarning(t *testing.T) {
	spec := objectSpec("self_probe", principal.ScopeRead)
	spec.SelfDescribing = true
	env := sealedEnvelope(t, invokeSealed(readingAgent(), t, fullSeatAuthority{},
		recordTool{spec: spec, records: []datasource.Record{recordAt(datasource.EntityContact, time.Time{}, true)}}))

	if _, present := warningNamed(env, warningRowScopeFiltered); present {
		t.Errorf("a self-describing answer carries the row-scope warning: %v", env.Warnings)
	}
}
