// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim_test

import (
	"bytes"
	"encoding/base64"
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
	notice := base64.StdEncoding.EncodeToString([]byte("This part was not kept"))[:20]
	if !bytes.Contains(out, []byte(notice)) {
		t.Error("the withheld part has no notice body, so a reader decoding it sees nothing or noise")
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
	if !bytes.Contains(out, []byte(partslim.PartWithheldHeader+": all")) {
		t.Errorf("an unsplittable original lost the marker saying why it is empty: %q", out)
	}
}

// A file sent 7bit is in the original verbatim. A base64 copy of it planted in
// the visible body must not be taken for the part: the splice would remove the
// copy and keep the file.
func TestWithholdPartsDoesNotTakeABase64CopyForASevenBitFile(t *testing.T) {
	file := []byte("payslip: salary 4200 EUR, account DE00 1234")
	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\n" +
		base64.StdEncoding.EncodeToString(file) + "\r\n" +
		"--b1\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=\"p.txt\"\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n\r\n" + string(file) + "\r\n--b1--\r\n")
	out, located := partslim.WithholdParts(raw, []partslim.WithheldPart{{Ordinal: 1, Body: file}})
	if located {
		t.Error("a base64 copy in the visible body was reported as the 7bit part")
	}
	if bytes.Contains(out, file) {
		t.Errorf("the 7bit file survived in the original: %q", out)
	}
}

// The same trap with the copy in a part that does declare base64: the header
// check admits it, so only the bytes surviving the splice can catch it.
func TestWithholdPartsCatchesASevenBitFileThatSurvivesTheSplice(t *testing.T) {
	file := []byte("payslip: salary 4200 EUR, account DE00 1234")
	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: application/octet-stream\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		base64.StdEncoding.EncodeToString(file) + "\r\n" +
		"--b1\r\nContent-Type: text/plain\r\nContent-Transfer-Encoding: 7bit\r\n\r\n" +
		string(file) + "\r\n--b1--\r\n")
	out, located := partslim.WithholdParts(raw, []partslim.WithheldPart{{Ordinal: 1, Body: file}})
	if located {
		t.Error("the splice removed the decoy and reported the 7bit file located")
	}
	if bytes.Contains(out, file) {
		t.Errorf("the 7bit file survived in the original: %q", out)
	}
}
