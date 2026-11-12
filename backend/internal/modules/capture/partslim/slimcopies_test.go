// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
)

// messageWithCopies carries one text body and the same attachment as copies
// separate parts, plus extra as plain text after them.
func messageWithCopies(pdf []byte, copies int, extra string) []byte {
	var raw strings.Builder
	raw.WriteString("MIME-Version: 1.0\r\n" +
		"Subject: The logo again\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"the visible body\r\n" + extra)
	for range copies {
		raw.WriteString("--b1\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\n\r\n" + wrap76(pdf) + "\r\n")
	}
	raw.WriteString("--b1--\r\n")
	return []byte(raw.String())
}

func TestStripStoredPartsCutsEveryCopyOfAFileItsPartsCarry(t *testing.T) {
	logo := attachmentBody(21)
	raw := messageWithCopies(logo, 3, "")
	parts := []partslim.StoredPart{storedPart(4, logo), storedPart(2, logo), storedPart(3, logo)}
	stripped, n, err := partslim.StripStoredParts(raw, parts)
	if err != nil || n != 3 {
		t.Fatalf("stripping: n=%d err=%v", n, err)
	}
	if bytes.Contains(stripped, []byte(wrap76(logo)[:200])) {
		t.Fatal("a copy of the file's encoding survived the strip")
	}
	var asked []int
	restored, err := partslim.RestoreStoredParts(stripped, func(ref partslim.PartRef) ([]byte, error) {
		asked = append(asked, ref.Ordinal)
		return logo, nil
	})
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if !bytes.Equal(restored, raw) {
		t.Errorf("restore was not byte-exact: got %d bytes, want %d", len(restored), len(raw))
	}
	if len(asked) != 3 || asked[0] != 2 || asked[1] != 3 || asked[2] != 4 {
		t.Errorf("the stanzas name ordinals %v; the copies appear as parts 2, 3, 4 in that order", asked)
	}
}

func TestStripStoredPartsLeavesCopiesAloneWhenTheFileAlsoAppearsOutsideItsParts(t *testing.T) {
	logo := attachmentBody(22)
	quoted := wrap76(logo) + "\r\n"
	raw := messageWithCopies(logo, 2, quoted)
	out, n, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(2, logo), storedPart(3, logo)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if n != 0 || !bytes.Equal(out, raw) {
		t.Errorf("stripped %d copies of a file that occurs three times for two parts; want none", n)
	}
}

func TestStripStoredPartsStillCutsADifferentFileBesideUnclaimedCopies(t *testing.T) {
	logo, pdf := attachmentBody(23), attachmentBody(24)
	raw := messageWithCopies(logo, 2, "--b1\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n\r\n"+wrap76(pdf)+"\r\n")
	_, n, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(2, pdf), storedPart(3, logo)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if n != 1 {
		t.Errorf("stripped %d parts; want the pdf only, since one part cannot claim two copies of the logo", n)
	}
}

// Two copies wrapped at different widths, and a text body that starts with one
// of the encodings. That encoding's count matches the parts, but one copy is
// not a base64 part, so nothing is cut.
func TestStripStoredPartsCutsNoTextBodyInThePlaceOfAPart(t *testing.T) {
	logo := attachmentBody(25)
	wrap72 := strings.ReplaceAll(wrap76(logo), "\r\n", "")
	var folded strings.Builder
	for at := 0; at < len(wrap72); at += 72 {
		if at > 0 {
			folded.WriteString("\r\n")
		}
		folded.WriteString(wrap72[at:min(at+72, len(wrap72))])
	}
	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\n" + wrap76(logo) + "\r\n" +
		"--b1\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\n\r\n" + wrap76(logo) + "\r\n" +
		"--b1\r\nContent-Type: image/png\r\nContent-Transfer-Encoding: base64\r\n\r\n" + folded.String() + "\r\n" +
		"--b1--\r\n")
	out, n, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(2, logo), storedPart(3, logo)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if n != 0 || !bytes.Equal(out, raw) {
		t.Errorf("stripped %d regions; one of them was the text body, so none may be cut", n)
	}
}
