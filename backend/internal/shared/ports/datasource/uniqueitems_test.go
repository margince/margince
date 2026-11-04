// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"strings"
	"testing"
)

type uniqueRecipient struct {
	IDs []string `json:"ids" unique:"items"`
}

type uniqueContact struct {
	Name string `json:"name"`
}

type uniqueBody struct {
	People     []uniqueContact   `json:"people" unique:"items"`
	Pointers   []*string         `json:"pointers" unique:"items"`
	Scopes     *[]string         `json:"scopes,omitempty" unique:"items"`
	Recipients []uniqueRecipient `json:"recipients"`
	Free       []string          `json:"free"`
	Pairs      [][]int           `json:"pairs" unique:"items"`
}

func TestRejectRepeatedItemsNamesTheListThatRepeatsAnItem(t *testing.T) {
	cases := map[string]struct {
		body uniqueBody
		want string
	}{
		"omitted list":               {body: uniqueBody{}},
		"each item once":             {body: uniqueBody{Scopes: &[]string{"read", "write"}}},
		"one item twice":             {body: uniqueBody{Scopes: &[]string{"read", "write", "read"}}, want: "scopes"},
		"inside a nested list":       {body: uniqueBody{Recipients: []uniqueRecipient{{IDs: []string{"a"}}, {IDs: []string{"b", "b"}}}}, want: "recipients[1].ids"},
		"list without the tag":       {body: uniqueBody{Free: []string{"x", "x"}}},
		"items no map can key":       {body: uniqueBody{Pairs: [][]int{{1, 2}, {1, 2}}}, want: "pairs"},
		"distinct uncomparable":      {body: uniqueBody{Pairs: [][]int{{1, 2}, {2, 1}}}},
		"equal structs":              {body: uniqueBody{People: []uniqueContact{{Name: "Ana"}, {Name: "Ana"}}}, want: "people"},
		"distinct structs":           {body: uniqueBody{People: []uniqueContact{{Name: "Ana"}, {Name: "Bo"}}}},
		"one value, two pointers":    {body: uniqueBody{Pointers: []*string{new("x"), new("x")}}, want: "pointers"},
		"two values behind pointers": {body: uniqueBody{Pointers: []*string{new("x"), new("y")}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := RejectRepeatedItems(&tc.body)
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("refused %q, want accepted", got.Field)
			case tc.want != "" && (got == nil || got.Field != tc.want):
				t.Fatalf("refusal %+v, want one naming %q", got, tc.want)
			}
		})
	}
}

func TestARepeatedItemRefusalNamesBothPositions(t *testing.T) {
	got := RejectRepeatedItems(&uniqueBody{Scopes: &[]string{"read", "write", "read"}})
	if got == nil {
		t.Fatal("accepted a repeated scope")
	}
	want := "`scopes` must be a list that names each item once, not a list whose item [2] repeats item [0]"
	if got.Error() != want {
		t.Fatalf("message = %q, want %q", got.Error(), want)
	}
}

// The provider seam shares the rule, so an agent cannot send what the REST
// body decode refuses.
func TestStrictDecodeRefusesARepeatedItem(t *testing.T) {
	var into uniqueBody
	err := StrictDecode([]byte(`{"scopes":["read","read"]}`), &into)
	if err == nil || !strings.Contains(err.Error(), "scopes") {
		t.Fatalf("StrictDecode = %v, want a refusal naming scopes", err)
	}
}
