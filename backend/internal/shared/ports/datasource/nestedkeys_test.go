// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

type nestedCatchAll struct {
	Known           string            `json:"known"`
	AdditionalProps map[string]string `json:"-"`
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
			err := RejectUnknownNestedKeys(json.RawMessage(tc.body), &nestedRoot{})
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			var unknown *UnknownFieldError
			require.ErrorAs(t, err, &unknown)
			assert.Equal(t, tc.want, unknown.Fields)
		})
	}
}
