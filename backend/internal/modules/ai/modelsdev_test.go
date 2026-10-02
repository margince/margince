// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func fixturePrices(t *testing.T) ModelsDevPrices {
	t.Helper()
	body, err := os.ReadFile("testdata/modelsdev-api.json")
	if err != nil {
		t.Fatal(err)
	}
	byKey, err := parseModelsDev(body)
	if err != nil {
		t.Fatalf("parsing the fixture: %v", err)
	}
	return ModelsDevPrices{byKey: byKey}
}

func fixtureEntry(t *testing.T, key, id string) modelsDevModel {
	t.Helper()
	m, ok := fixturePrices(t).entry(key, id)
	if !ok {
		t.Fatalf("fixture has no %s/%s", key, id)
	}
	return m
}

func TestModelsDevFiguresAreReadAsTheSheetsDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"1.5", "1.500000", true},
		{"0.025", "0.025000", true},
		{"3", "3.000000", true},
		{"1e-6", "0.000001", true},
		{"0", "0.000000", true},
		{"1e-7", "", false},
		{"0.1234567", "0.123457", true},
		{"-1", "", false},
		{"free", "", false},
		{"1e999999999", "", false},
		{"2000000", "", false},
	}
	for _, tc := range cases {
		got, ok := usdPerMTok(json.Number(tc.in))
		if got != tc.want || ok != tc.ok {
			t.Errorf("usdPerMTok(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAMissingFigureKeepsThePriceInForce(t *testing.T) {
	entry := fixtureEntry(t, "google", "gemini-3.5-flash")
	current := ModelRateRow{InputUsd: "1.4", OutputUsd: "8", CacheReadUsd: "0.1", CacheWriteUsd: "0.5"}
	got, ok := entry.price(&current)
	if !ok {
		t.Fatal("a priced entry was refused")
	}
	if got.InputUsd != "1.500000" || got.OutputUsd != "9.000000" || got.CacheReadUsd != "0.150000" {
		t.Errorf("stated figures = %+v, want 1.5 / 9 / 0.15", got)
	}
	if got.CacheWriteUsd != "0.5" {
		t.Errorf("cache write = %q, want the 0.5 in force: the catalogue states none", got.CacheWriteUsd)
	}
	fresh, ok := entry.price(nil)
	if !ok || fresh.CacheWriteUsd != "0" {
		t.Errorf("a model with no row yet = %+v, %v; want cache write 0", fresh, ok)
	}
}

func TestAModelWithNoCostOrNoInputIsUnpriced(t *testing.T) {
	if _, ok := fixtureEntry(t, "google", "gemma-4-31b-it").price(nil); ok {
		t.Error("an entry with no cost was priced — it would be written as free")
	}
	out := json.Number("2")
	noInput := modelsDevModel{ID: "x", Cost: &modelsDevCost{Output: &out}}
	if _, ok := noInput.price(nil); ok {
		t.Error("an entry with no input price was priced")
	}
}

func TestTheCatalogueSaysWhatAModelIsFor(t *testing.T) {
	cases := []struct {
		key, id string
		want    Lane
		ok      bool
	}{
		{"google", "gemini-3.5-flash", LaneChat, true},
		{"google", "gemini-embedding-001", LaneEmbeddings, true},
		{"openai", "text-embedding-3-small", LaneEmbeddings, true},
		{"google", "gemini-2.5-flash-image", "", false},
		{"google", "gemini-2.5-flash-preview-tts", "", false},
		{"openai", "gpt-image-1", "", false},
	}
	for _, tc := range cases {
		got, ok := fixtureEntry(t, tc.key, tc.id).lane()
		if got != tc.want || ok != tc.ok {
			t.Errorf("%s/%s lane = %q, %v; want %q, %v", tc.key, tc.id, got, ok, tc.want, tc.ok)
		}
	}
}

func TestModelsDevFailsOpenAndCachesOnlyASuccess(t *testing.T) {
	body, err := os.ReadFile("testdata/modelsdev-api.json")
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &fakeCatalogueFetcher{err: errors.New("dial: refused")}
	clock := &fixedClock{now: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)}
	c := NewModelsDevCatalogueOver(fetcher, clock)

	if got := c.Prices(context.Background()); !got.Unreachable {
		t.Fatal("a failed read was served as a catalogue")
	}
	fetcher.err, fetcher.body = nil, body
	if got := c.Prices(context.Background()); got.Unreachable {
		t.Fatal("the read after a failure was not retried")
	}
	clock.now = clock.now.Add(14 * time.Minute)
	if got := c.Prices(context.Background()); got.Unreachable || fetcher.calls != 2 {
		t.Errorf("within the TTL: unreachable=%v after %d fetches, want the cache and 2", got.Unreachable, fetcher.calls)
	}
}

func TestAnEmptyModelsDevAnswerIsUnreachable(t *testing.T) {
	c := NewModelsDevCatalogueOver(&fakeCatalogueFetcher{body: []byte(`{}`)}, &fixedClock{})
	if !c.Prices(context.Background()).Unreachable {
		t.Error("a catalogue naming no provider was served as one")
	}
}
