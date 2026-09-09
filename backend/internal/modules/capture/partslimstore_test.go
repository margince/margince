// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What the sweep will and will not vouch for.
//
// The filter is the safety boundary: a part it keeps has its bytes removed from
// the provider original, so every arm that rejects one is a copy this does not
// destroy. Tested against a real in-memory store rather than a stub of one,
// because the arms that matter are the store's own answers — absent, and the
// wrong length.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/margince/margince/backend/internal/platform/blobstore"
)

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func TestProvePartsKeepsOnlyWhatTheStoreVouchesFor(t *testing.T) {
	ctx := context.Background()
	blob := blobstore.NewMemory()
	good := []byte("%PDF-1.4 the real bytes\n")
	wrongLength := []byte("%PDF-1.4 shorter\n")

	if err := blob.Put(ctx, "ws/attachment/good", bytes.NewReader(good),
		int64(len(good)), "application/pdf"); err != nil {
		t.Fatalf("seeding the good object: %v", err)
	}
	if err := blob.Put(ctx, "ws/attachment/short", bytes.NewReader(wrongLength),
		int64(len(wrongLength)), "application/pdf"); err != nil {
		t.Fatalf("seeding the mismatched object: %v", err)
	}

	store := NewPartSlimStore(nil, blob)
	proved, unproved := store.proveParts(ctx, []CandidatePart{
		{Ordinal: 1, StorageKey: "ws/attachment/good", ByteSize: int64(len(good)), Checksum: digest(good)},
		{Ordinal: 2, StorageKey: "", ByteSize: int64(len(good)), Checksum: digest(good)},
		{Ordinal: 3, StorageKey: "ws/attachment/short", ByteSize: int64(len(good)), Checksum: digest(good)},
		{Ordinal: 4, StorageKey: "ws/attachment/absent", ByteSize: int64(len(good)), Checksum: digest(good)},
		{Ordinal: 5, StorageKey: "ws/attachment/good", ByteSize: int64(len(good)), Checksum: ""},
	})

	if len(proved) != 1 || proved[0].Ordinal != 1 {
		t.Fatalf("proved %+v, want only ordinal 1", proved)
	}
	if !bytes.Equal(proved[0].Body, good) {
		t.Errorf("the proved part carries the wrong octets")
	}
	if unproved != 4 {
		t.Errorf("unproved = %d, want 4", unproved)
	}
}

// With no store wired there is no proof to have, so nothing is vouched for.
// This is the arm that keeps an unwired deployment from removing attachment
// bytes that exist nowhere else.
func TestProvePartsVouchesForNothingWithoutAStore(t *testing.T) {
	store := NewPartSlimStore(nil, nil)
	proved, unproved := store.proveParts(context.Background(), []CandidatePart{
		{Ordinal: 1, StorageKey: "ws/attachment/good", ByteSize: 9, Checksum: digest([]byte("x"))},
		{Ordinal: 2, StorageKey: "ws/attachment/also", ByteSize: 9, Checksum: digest([]byte("y"))},
	})
	if len(proved) != 0 {
		t.Errorf("proved %d parts with no object store", len(proved))
	}
	if unproved != 2 {
		t.Errorf("unproved = %d, want 2", unproved)
	}
}

// The aggregate the candidates query builds has to land in the struct the
// filter reads. A renamed key would otherwise leave a zero-valued field that
// the filter rejects as unprovable — a silent no-op sweep rather than an error.
func TestCandidatePartReadsTheAggregateShape(t *testing.T) {
	var part CandidatePart
	if err := part.UnmarshalJSON([]byte(
		`{"ordinal":3,"key":"ws/attachment/x","bytes":4096,"sum":"abc123"}`)); err != nil {
		t.Fatalf("reading the aggregate: %v", err)
	}
	if part.Ordinal != 3 || part.StorageKey != "ws/attachment/x" ||
		part.ByteSize != 4096 || part.Checksum != "abc123" {
		t.Errorf("read %+v from the aggregate", part)
	}
}
