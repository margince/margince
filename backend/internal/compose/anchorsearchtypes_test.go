// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/search"
)

// An intent tool resolves a record_name on the ranked engine, so every type it
// anchors on must be one the engine searches. A type the engine does not know
// would refuse every name of that type as an unknown search type.
func TestEveryIntentAnchorTypeIsOneTheEngineSearches(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	for _, tool := range []string{"catch_me_up_on", "prep_for_meeting"} {
		spec, ok := registry.Spec(tool)
		if !ok {
			t.Fatalf("%s is not registered, so its anchor types cannot be checked", tool)
		}
		var schema struct {
			Properties struct {
				RecordType struct {
					Enum []string `json:"enum"`
				} `json:"record_type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil || len(schema.Properties.RecordType.Enum) == 0 {
			t.Fatalf("%s publishes no record_type enum (%v)", tool, err)
		}
		for _, recordType := range schema.Properties.RecordType.Enum {
			// The store has no database: a type it knows fails reaching one,
			// and only an unknown type is refused as a bad `types`.
			_, err := (&search.Store{}).Search(context.Background(), search.Input{Query: "x", Types: []string{recordType}})
			if bad, ok := errors.AsType[*search.BadQueryError](err); ok && bad.Field == "types" {
				t.Errorf("%s anchors on %q, which the ranked engine does not search: %v", tool, recordType, err)
			}
		}
	}
}
