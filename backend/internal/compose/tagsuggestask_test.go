// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// jsonKeysOf reads the JSON keys a struct decodes from its own tags.
func jsonKeysOf[T any]() []string {
	var keys []string
	for _, f := range reflect.VisibleFields(reflect.TypeFor[T]()) {
		keys = append(keys, f.Tag.Get("json"))
	}
	slices.Sort(keys)
	return keys
}

// The schema and the decoder spell the reply's keys separately, so a key
// renamed on one side alone would decode every reply as empty.
func TestTheReplyDecoderReadsTheKeysTheSchemaDeclares(t *testing.T) {
	type node struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Items      json.RawMessage            `json:"items"`
	}
	var payload node
	if err := json.Unmarshal(tagSuggestSchema([]string{ids.NewV7().String()}, []string{ids.NewV7().String()}), &payload); err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	if got, want := slices.Sorted(maps.Keys(payload.Properties)), jsonKeysOf[tagSuggestPayload](); !slices.Equal(got, want) {
		t.Fatalf("the schema declares %v and the payload decodes %v", got, want)
	}
	var items, answer node
	if err := json.Unmarshal(payload.Properties[tagSuggestItemsKey], &items); err != nil {
		t.Fatalf("reading the items array: %v", err)
	}
	if err := json.Unmarshal(items.Items, &answer); err != nil {
		t.Fatalf("reading one answer: %v", err)
	}
	if got, want := slices.Sorted(maps.Keys(answer.Properties)), jsonKeysOf[tagSuggestAnswer](); !slices.Equal(got, want) {
		t.Fatalf("the schema declares %v per item and the answer decodes %v", got, want)
	}
}
