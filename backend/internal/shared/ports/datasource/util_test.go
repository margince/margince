// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package datasource

import (
	"encoding/json"
	"errors"
	"testing"
)

// A patch field key must match a contract field name byte-for-byte.
// encoding/json matches struct fields case-insensitively, so without this
// gate `{"FULL_NAME":…}` would decode into `full_name` and write the
// column — while the human-edit-precedence probe, matching audit keys
// case-sensitively in jsonb, cleared the same patch as touching no
// human-owned field. That gap let an agent overwrite a human-typed value
// through the 🟢 path under a differently-cased key.
func TestStrictDecodeRefusesCaseVariantFieldKeys(t *testing.T) {
	type leadLike struct {
		FullName *string `json:"full_name,omitempty"`
		Score    *int    `json:"score,omitempty"`
	}

	var exact leadLike
	if err := StrictDecode(json.RawMessage(`{"full_name":"Ada"}`), &exact); err != nil {
		t.Fatalf("exact-case key must decode: %v", err)
	}
	if exact.FullName == nil || *exact.FullName != "Ada" {
		t.Fatalf("exact-case key did not populate the field: %+v", exact)
	}

	for _, variant := range []string{
		`{"FULL_NAME":"attacker"}`,
		`{"Full_Name":"attacker"}`,
		`{"Score":9}`,
	} {
		var got leadLike
		err := StrictDecode(json.RawMessage(variant), &got)
		if err == nil {
			t.Fatalf("case-variant key %s was accepted; the store would write it while the probe cleared it", variant)
		}
		if _, ok := errors.AsType[*FieldDecodeError](err); !ok {
			t.Fatalf("case-variant key %s: want FieldDecodeError (maps to 422), got %T: %v", variant, err, err)
		}
	}
}

// A target with an AdditionalProperties catch-all is left to the split decode,
// which lifts its cf_ keys and holds the rest to the closed rules. A plain
// ignored `json:"-"` field is no catch-all, so such a target is checked here.
func TestRejectNonCanonicalKeysLeavesACatchAllTargetToTheSplitDecode(t *testing.T) {
	type withMapCatchAll struct {
		FullName             *string        `json:"full_name,omitempty"`
		AdditionalProperties map[string]any `json:"-"`
	}
	var lax withMapCatchAll
	if err := RejectNonCanonicalKeys(json.RawMessage(`{"ful_name":"x"}`), &lax); err != nil {
		t.Fatalf("a catch-all target is the split decode's: %v", err)
	}
	err := StrictDecode(json.RawMessage(`{"full_name":"Ada","cf_tier":"gold"}`), &lax)
	if err != nil || lax.AdditionalProperties["cf_tier"] != "gold" {
		t.Fatalf("StrictDecode did not route the target through the split decode: %v %+v", err, lax)
	}
	err = StrictDecode(json.RawMessage(`{"ful_name":"x"}`), &lax)
	if _, ok := errors.AsType[*FieldDecodeError](err); !ok {
		t.Fatalf("a misspelt key was answered %v, want a FieldDecodeError", err)
	}

	type withIgnoredField struct {
		FullName *string `json:"full_name,omitempty"`
		internal int     `json:"-"`
	}
	var strict withIgnoredField
	_ = strict.internal
	if err := RejectNonCanonicalKeys(json.RawMessage(`{"cf_tier":"x"}`), &strict); err == nil {
		t.Fatal("a plain json:\"-\" ignored field is no catch-all, so a cf_ key is unknown there")
	}
}

// encoding/json promotes an embedded struct's fields to the parent, so the
// exact-key gate must walk into anonymous struct fields — otherwise a
// promoted key is falsely rejected as unknown, and an embedded catch-all
// is missed. No contract type embeds today; this pins the behavior before
// one does.
func TestRejectNonCanonicalKeysWalksEmbeddedFields(t *testing.T) {
	type base struct {
		OwnerId *string `json:"owner_id,omitempty"`
	}
	type child struct {
		base
		FullName *string `json:"full_name,omitempty"`
	}
	var c child
	if err := RejectNonCanonicalKeys(json.RawMessage(`{"owner_id":"u1","full_name":"Ada"}`), &c); err != nil {
		t.Fatalf("promoted embedded key must be accepted: %v", err)
	}
	if err := RejectNonCanonicalKeys(json.RawMessage(`{"OWNER_ID":"u1"}`), &c); err == nil {
		t.Fatal("a case-variant of a promoted embedded key must be rejected")
	}
}
