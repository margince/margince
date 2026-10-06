// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

type nestedLeaf struct {
	Only []string `json:"only,omitempty"`
}

type nestedBase struct {
	Name string `json:"name"`
}

type nestedOwnPolicy struct{ kept string }

func (n *nestedOwnPolicy) UnmarshalJSON(raw []byte) error {
	n.kept = string(raw)
	return nil
}

// nestedCatchAll has the shape of a generated additionalProperties:true type:
// the extra members land in a map, through a decoder of its own.
type nestedCatchAll struct {
	Known           string            `json:"known"`
	AdditionalProps map[string]string `json:"-"`
}

func (c *nestedCatchAll) UnmarshalJSON(raw []byte) error {
	var members map[string]string
	if err := json.Unmarshal(raw, &members); err != nil {
		return err
	}
	c.Known = members["known"]
	delete(members, "known")
	c.AdditionalProps = members
	return nil
}

type nestedRoot struct {
	nestedBase
	Upstream *nestedLeaf                `json:"upstream,omitempty"`
	Tiers    map[string]nestedLeaf      `json:"tiers,omitempty"`
	Items    []nestedLeaf               `json:"items,omitempty"`
	Own      *nestedOwnPolicy           `json:"own,omitempty"`
	Open     *nestedCatchAll            `json:"open,omitempty"`
	Free     map[string]json.RawMessage `json:"free,omitempty"`
}

func TestRejectUnknownNestedKeys(t *testing.T) {
	cases := map[string]struct {
		body string
		want []string
	}{
		"clean document":                 {`{"name":"a","upstream":{"only":["x"]},"tiers":{"t":{"only":[]}},"items":[{}]}`, nil},
		"unknown key in a nested object": {`{"upstream":{"only":["x"],"zdr":true}}`, []string{"upstream.zdr"}},
		"a case variant of a nested key": {`{"upstream":{"ONLY":[]}}`, []string{"upstream.ONLY"}},
		"under a map value":              {`{"tiers":{"cheap":{"nope":1}}}`, []string{"tiers.cheap.nope"}},
		"in a slice element":             {`{"items":[{},{"nope":1}]}`, []string{"items[1].nope"}},
		"every unknown key, sorted":      {`{"upstream":{"b":1,"a":1}}`, []string{"upstream.a", "upstream.b"}},
		"a type with its own decoder":    {`{"own":{"anything":1}}`, nil},
		"a catch-all type":               {`{"open":{"known":"k","anything":1}}`, nil},
		"an untyped map":                 {`{"free":{"anything":{"deeper":1}}}`, nil},
		"a null nested object":           {`{"upstream":null}`, nil},
		"a wrong shape is the decoder's": {`{"upstream":[1],"items":{"a":1}}`, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := RejectUnknownNestedKeys(json.RawMessage(tc.body), &nestedRoot{}, nil)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("refused %s: %v", tc.body, err)
				}
				return
			}
			var unknown *UnknownFieldError
			if !errors.As(err, &unknown) {
				t.Fatalf("answered %v, want an unknown-field refusal", err)
			}
			if !reflect.DeepEqual(unknown.Fields, tc.want) {
				t.Errorf("named %v, want %v", unknown.Fields, tc.want)
			}
		})
	}
}

func TestAPathAnotherLayerJudgesIsNotEntered(t *testing.T) {
	owned := func(path string) bool { return path == "upstream" }
	if err := RejectUnknownNestedKeys(json.RawMessage(`{"upstream":{"nope":1}}`), &nestedRoot{}, owned); err != nil {
		t.Errorf("refused a path the caller claimed: %v", err)
	}
	if err := RejectUnknownNestedKeys(json.RawMessage(`{"upstream":{"nope":1}}`), &nestedRoot{}, func(string) bool { return false }); err == nil {
		t.Error("a claim of nothing let an unknown key through")
	}
}
