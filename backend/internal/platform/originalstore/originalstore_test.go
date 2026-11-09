// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package originalstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

func TestStoredOriginalsRequireTheirWorkspaceAndExactBytes(t *testing.T) {
	ws := ids.NewV7()
	ctx := principal.WithWorkspaceID(t.Context(), ws)
	blob := blobstore.NewMemory()
	ctx = Bind(ctx, nil, blob)
	root, err := prefix(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"message":"verbatim"}`)
	sum := sha256.Sum256(payload)
	ref := Reference{Key: root + "original", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(payload))}
	put := func(content []byte) {
		t.Helper()
		if err := blob.Put(ctx, ref.Key, bytes.NewReader(content), int64(len(content)), "application/json"); err != nil {
			t.Fatal(err)
		}
	}
	put(payload)
	got, err := Read(ctx, ref)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("stored original changed: %q, %v", got, err)
	}
	other := principal.WithWorkspaceID(ctx, ids.NewV7())
	if _, err := Read(other, ref); err == nil {
		t.Fatal("a different workspace read the stored original")
	}
	if err := Delete(other, ref); err == nil {
		t.Fatal("a different workspace deleted the stored original")
	}
	put([]byte(`{"message":"modified"}`))
	if _, err := Read(ctx, ref); err == nil {
		t.Fatal("same-sized modified bytes passed the integrity check")
	}
	put(payload[:len(payload)-1])
	if _, err := Read(ctx, ref); err == nil {
		t.Fatal("truncated bytes passed the integrity check")
	}
	if err := Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, ref); err != nil {
		t.Fatalf("retrying a delete failed: %v", err)
	}
}
