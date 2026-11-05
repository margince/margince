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
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
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

// A provider object carrying its own "encoding" key is not one of this file's
// envelopes.
//
// rawCapturePayload stores a valid JSON object unchanged, so such a payload
// reaches DecodeStoredOriginal intact. Read as an envelope it would hand back
// the contents of a "data" key that means something else — or nothing at all
// when there is no such key — and participant replay would re-read the wrong
// bytes or call a readable message unreadable.
func TestDecodeStoredOriginalDoesNotMistakeAProviderObjectForAnEnvelope(t *testing.T) {
	for _, payload := range []string{
		`{"encoding":"base64","kind":"calendar#event","summary":"Quarterly figures"}`,
		`{"encoding":"base64"}`,
		`{"encoding":"base64","data":"JVBERg==","summary":"one key too many"}`,
	} {
		got, err := DecodeStoredOriginal([]byte(payload))
		if err != nil {
			t.Errorf("DecodeStoredOriginal(%s) errored: %v", payload, err)
			continue
		}
		if string(got) != payload {
			t.Errorf("DecodeStoredOriginal(%s) = %q, want the payload unchanged", payload, got)
		}
	}
}

// The real envelope still decodes.
func TestDecodeStoredOriginalUnwrapsItsOwnEnvelope(t *testing.T) {
	got, err := DecodeStoredOriginal([]byte(`{"encoding":"base64","data":"JVBERi0xLjQK"}`))
	if err != nil {
		t.Fatalf("decoding this file's own envelope: %v", err)
	}
	if string(got) != "%PDF-1.4\n" {
		t.Errorf("decoded %q, want the original bytes", got)
	}
}

// attemptPDF is an attachment body big enough that removing it saves bytes.
func attemptPDF(seed byte) []byte {
	body := make([]byte, 8192)
	copy(body, "%PDF-1.4\n")
	for i := len("%PDF-1.4\n"); i < len(body); i++ {
		body[i] = byte(int(seed)*13 + i*37%251)
	}
	return body
}

func attemptDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func attemptWrapped(body []byte) string {
	b64 := base64.StdEncoding.EncodeToString(body)
	var out strings.Builder
	for at := 0; at < len(b64); at += 76 {
		if at > 0 {
			out.WriteString("\r\n")
		}
		out.WriteString(b64[at:min(at+76, len(b64))])
	}
	return out.String()
}

func attemptMessage(pdf []byte) []byte {
	return []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"the numbers themselves\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		attemptWrapped(pdf) + "\r\n" +
		"--b1--\r\n")
}

func attemptPayload(t *testing.T, raw []byte) []byte {
	t.Helper()
	payload, err := EncodeStoredOriginal(raw)
	if err != nil {
		t.Fatalf("spelling the original the way the sink does: %v", err)
	}
	return payload
}

// Every answer attemptSlim can give, and the reason it gives with it.
//
// Six of its seven outcomes leave the payload whole. The row they stamp is
// read to decide what to do about an original too large to slim. An outcome
// naming the wrong reason would send that decision the wrong way.
func TestAttemptSlimSaysWhyNothingCameOut(t *testing.T) {
	pdf := attemptPDF(5)
	proved := []partslim.StoredPart{{
		Ordinal: 1, Sha256: attemptDigest(pdf), Bytes: int64(len(pdf)),
		StorageKey: "ws/attachment/one", Body: pdf,
	}}

	t.Run("an original it can rewrite", func(t *testing.T) {
		got := attemptSlim(attemptPayload(t, attemptMessage(pdf)), proved)
		if got.outcome != outcomeSlimmed {
			t.Fatalf("records %q, want %q", got.outcome, outcomeSlimmed)
		}
		if got.saved <= 0 {
			t.Errorf("saved %d bytes, want more than zero", got.saved)
		}
		if len(got.rewritten) == 0 {
			t.Error("reports a slim and offers no bytes to write")
		}
	})

	t.Run("a payload that only looks like the spelling", func(t *testing.T) {
		// It opens like the JSON string the sink writes and is not one, which
		// is the case the column cannot be read at all. A payload in no
		// spelling is handed back whole by design, so it reaches stripping
		// instead.
		got := attemptSlim([]byte(`"unterminated`), proved)
		if got.outcome != outcomeUnreadableSpelling {
			t.Errorf("records %q, want %q", got.outcome, outcomeUnreadableSpelling)
		}
	})

	t.Run("a part whose bytes disagree with its row", func(t *testing.T) {
		lying := []partslim.StoredPart{{
			Ordinal: 1, Sha256: attemptDigest([]byte("something else")),
			Bytes: int64(len(pdf)), StorageKey: "ws/attachment/one", Body: pdf,
		}}
		got := attemptSlim(attemptPayload(t, attemptMessage(pdf)), lying)
		if got.outcome != outcomeUnrewritableOriginal {
			t.Errorf("records %q, want %q", got.outcome, outcomeUnrewritableOriginal)
		}
	})

	t.Run("a part the original does not carry", func(t *testing.T) {
		elsewhere := attemptPDF(9)
		absent := []partslim.StoredPart{{
			Ordinal: 1, Sha256: attemptDigest(elsewhere),
			Bytes: int64(len(elsewhere)), StorageKey: "ws/attachment/two",
			Body: elsewhere,
		}}
		got := attemptSlim(attemptPayload(t, attemptMessage(pdf)), absent)
		if got.outcome != outcomeNothingCameOut {
			t.Errorf("records %q, want %q", got.outcome, outcomeNothingCameOut)
		}
		if got.rewritten != nil {
			t.Error("offers bytes to write for an original it did not rewrite")
		}
	})

	t.Run("nothing proved", func(t *testing.T) {
		got := attemptSlim(attemptPayload(t, attemptMessage(pdf)), nil)
		if got.outcome != outcomeNothingCameOut {
			t.Errorf("records %q, want %q", got.outcome, outcomeNothingCameOut)
		}
	})
}
