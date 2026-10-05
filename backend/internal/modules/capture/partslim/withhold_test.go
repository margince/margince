// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim_test

import (
	"bytes"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
)

// A withheld part leaves no trace of its bytes, and the rest of the message,
// visible body included, is kept as it was.
func TestWithholdPartsRemovesTheBytesAndKeepsTheBody(t *testing.T) {
	pdf := attachmentBody(11)
	raw := messageWith(pdf)
	out, located := partslim.WithholdParts(raw, []partslim.WithheldPart{{Ordinal: 1, Body: pdf}})
	if !located {
		t.Fatal("the part was not located, so the message fell back to its headers")
	}
	if bytes.Contains(out, []byte(wrap76(pdf))) {
		t.Error("the withheld part's encoded bytes are still in the original")
	}
	for _, kept := range []string{"the visible body", "filename=\"figures.pdf\"", partslim.PartWithheldHeader + ": part:1"} {
		if !bytes.Contains(out, []byte(kept)) {
			t.Errorf("the withheld original lost %q", kept)
		}
	}
	if partslim.IsSlimmed(out) {
		t.Error("a withheld part reads as a slimmed one, so a restore would try to fetch bytes that were never kept")
	}
}

// A part the locator cannot find exactly once — here the same file sent twice —
// cuts the message to its headers rather than keeping the bytes.
func TestWithholdPartsFallsBackToTheHeadersWhenAPartIsNotLocated(t *testing.T) {
	pdf := attachmentBody(12)
	one := messageWith(pdf)
	twice := append(append([]byte(nil), one...), one...)
	out, located := partslim.WithholdParts(twice, []partslim.WithheldPart{{Ordinal: 1, Body: pdf}})
	if located {
		t.Fatal("a file present twice was reported located")
	}
	if bytes.Contains(out, []byte(wrap76(pdf))) {
		t.Error("the fallback kept the part's bytes")
	}
	if !bytes.HasPrefix(out, []byte("MIME-Version: 1.0\r\nSubject: Quarterly figures\r\n")) {
		t.Errorf("the fallback lost the message headers: %q", out)
	}
}

// An original with no blank line cannot be split into header and body, so
// nothing of it is kept but the marker.
func TestHeadersOnlyKeepsNothingOfAnUnsplittableOriginal(t *testing.T) {
	out := partslim.HeadersOnly([]byte("Subject: no blank line anywhere JVBERi0xLjQ="))
	if bytes.Contains(out, []byte("JVBERi0xLjQ=")) {
		t.Errorf("an unsplittable original kept its bytes: %q", out)
	}
}
