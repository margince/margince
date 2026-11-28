// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package originalstore

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestInlineOriginalsCannotSupplyObjectReferences(t *testing.T) {
	original := json.RawMessage(`{"inline":"forged","object":{"key":"other/raw-original/secret","bytes":1,"sha256":"bad"}}`)
	wrapper, err := json.Marshal(struct {
		Inline json.RawMessage `json:"inline"`
		Object *Reference      `json:"object"`
	}{Inline: original})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(t.Context(), wrapper)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("provider JSON was treated as metadata: %q, %v", got, err)
	}
}

func TestTheInlineBoundaryRequiresDurableStorageBeyondIt(t *testing.T) {
	payload := []byte(`"` + strings.Repeat("a", InlineLimit-2) + `"`)
	inline, ref, err := Prepare(t.Context(), payload)
	if err != nil || ref != nil || !bytes.Equal(inline, payload) {
		t.Fatalf("inline original changed: ref=%v err=%v", ref, err)
	}
	payload = append(payload[:len(payload)-1], 'a', '"')
	if _, _, err := Prepare(t.Context(), payload); err == nil {
		t.Fatal("oversized original was accepted without object storage")
	}
	if _, _, err := Prepare(t.Context(), []byte(strings.Repeat("x", InlineLimit+1))); err == nil {
		t.Fatal("offloading made malformed JSON admissible")
	}
}
