// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// customFieldShape is the generated pattern: closed core fields beside an
// AdditionalProperties map, and an UnmarshalJSON that would admit anything.
type customFieldShape struct {
	Kind                 string             `json:"kind"`
	Links                *[]customFieldLink `json:"links,omitempty"`
	AdditionalProperties map[string]any     `json:"-"`
}

type customFieldLink struct {
	EntityID string `json:"entity_id"`
}

func (c *customFieldShape) UnmarshalJSON([]byte) error { return nil }

func TestACustomFieldBodyCarriesItsCfKeysAndRefusesEveryOtherStranger(t *testing.T) {
	var got customFieldShape
	if err := StrictCustomFieldDecode(json.RawMessage(`{"kind":"note","cf_visit":"on_site","cf_count":2}`), &got); err != nil {
		t.Fatalf("a body with cf_ keys was refused: %v", err)
	}
	if got.Kind != "note" || got.AdditionalProperties["cf_visit"] != "on_site" || got.AdditionalProperties["cf_count"] != float64(2) {
		t.Fatalf("decoded %+v, want the kind and both cf_ values", got)
	}

	for body, want := range map[string]string{
		`{"kind":"note","subjekt":"x"}`:                    `unknown field "subjekt"`,
		`{"kind":"note","cf_":"x"}`:                        `unknown field "cf_"`,
		`{"kind":"note","links":[{"company_id":"x"}]}`:     "each item is {entity_id: string}",
		`{"kind":"note","cf_visit":"x","Kind":"shouting"}`: `unknown field "Kind"`,
	} {
		var into customFieldShape
		err := StrictCustomFieldDecode(json.RawMessage(body), &into)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s decoded with %v, want a refusal naming %s", body, err, want)
		}
	}
}

func TestClaimKeysAnswersTheFirstClaimedKeyInKeyOrder(t *testing.T) {
	errTotal := errors.New("a total")
	claim := func(key string) error {
		if strings.HasSuffix(key, "_minor") {
			return fmt.Errorf("%s: %w", key, errTotal)
		}
		return nil
	}
	err := ClaimKeys(json.RawMessage(`{"z_minor":1,"name":"x","a_minor":2}`), claim)
	if !errors.Is(err, errTotal) || !strings.HasPrefix(err.Error(), "a_minor") {
		t.Fatalf("got %v, want the refusal of a_minor, first in key order", err)
	}
	for _, body := range []string{`{"name":"x"}`, `[1]`, `null`, `not json`} {
		if err := ClaimKeys(json.RawMessage(body), claim); err != nil {
			t.Errorf("%s was claimed: %v", body, err)
		}
	}
}
