// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package partslim_test

// The strip removes only the octets it located, and the restore puts back
// exactly what was removed.

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
)

// attachmentBody is an attachment large enough to be worth removing. Real ones
// average ~250 kB and run to 22 MB; the content varies per seed so two fixtures
// cannot accidentally share an encoding.
func attachmentBody(seed byte) []byte {
	body := make([]byte, 4096)
	copy(body, "%PDF-1.4\n")
	for i := len("%PDF-1.4\n"); i < len(body); i++ {
		body[i] = byte(int(seed)*7 + i*31%251)
	}
	return body
}

// wrap76 folds base64 the way every provider in the corpus does.
func wrap76(body []byte) string {
	b64 := base64.StdEncoding.EncodeToString(body)
	var out strings.Builder
	for at := 0; at < len(b64); at += 76 {
		if at > 0 {
			out.WriteString("\r\n")
		}
		end := at + 76
		if end > len(b64) {
			end = len(b64)
		}
		out.WriteString(b64[at:end])
	}
	return out.String()
}

// messageWith is a multipart/mixed carrying one visible text body and one
// base64 attachment.
func messageWith(pdf []byte) []byte {
	return []byte("MIME-Version: 1.0\r\n" +
		"Subject: Quarterly figures\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"the visible body\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"figures.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		wrap76(pdf) + "\r\n" +
		"--b1--\r\n")
}

func storedPart(ordinal int, body []byte) partslim.StoredPart {
	return partslim.StoredPart{
		Ordinal: ordinal, Sha256: partslim.Sha256HexForTest(body),
		Bytes: int64(len(body)), StorageKey: "ws/attachment/aaa", Body: body,
	}
}

func TestStripStoredPartsRemovesOnlyTheLocatedOctets(t *testing.T) {
	pdf := attachmentBody(1)
	raw := messageWith(pdf)
	out, stripped, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if stripped != 1 {
		t.Fatalf("stripped %d parts, want 1", stripped)
	}
	if !bytes.Contains(out, []byte(partslim.PartStoredHeader+": part:1")) {
		t.Errorf("the stanza does not name the part")
	}
	if bytes.Contains(out, []byte(wrap76(pdf))) {
		t.Errorf("the encoded body survived the strip")
	}
	if !bytes.Contains(out, []byte("the visible body")) {
		t.Errorf("the text part was collateral damage")
	}
	// The provider's own fields survive, in order, ahead of the appended run.
	if !bytes.Contains(out, []byte("Content-Type: application/pdf\r\n"+
		"Content-Disposition: attachment; filename=\"figures.pdf\"\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		partslim.PartStoredHeader+": part:1\r\n")) {
		t.Errorf("the provider's header fields were reordered or rewritten:\n%s", firstKB(out))
	}
	if len(out) >= len(raw) {
		t.Errorf("strip did not shrink the message: %d -> %d", len(raw), len(out))
	}
}

func TestRestoreStoredPartsIsByteExact(t *testing.T) {
	pdf := attachmentBody(2)
	raw := messageWith(pdf)
	stripped, n, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(1, pdf)})
	if err != nil || n != 1 {
		t.Fatalf("stripping: n=%d err=%v", n, err)
	}
	restored, err := partslim.RestoreStoredParts(stripped, func(ref partslim.PartRef) ([]byte, error) {
		if ref.StorageKey != "ws/attachment/aaa" || ref.Ordinal != 1 {
			t.Errorf("fetch asked for %+v", ref)
		}
		return pdf, nil
	})
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if !bytes.Equal(restored, raw) {
		t.Errorf("restore was not byte-exact: got %d bytes, want %d", len(restored), len(raw))
	}
}

func TestRestoreStoredPartsRefusesAChangedObject(t *testing.T) {
	pdf := attachmentBody(3)
	stripped, _, err := partslim.StripStoredParts(messageWith(pdf),
		[]partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	tampered := append([]byte(nil), pdf...)
	tampered[0] = 'X'
	if _, err := partslim.RestoreStoredParts(stripped, func(partslim.PartRef) ([]byte, error) {
		return tampered, nil
	}); err == nil {
		t.Fatal("restore accepted an object whose digest did not match")
	}
}

func TestRestoreStoredPartsRefusesAResizedObject(t *testing.T) {
	pdf := attachmentBody(4)
	stripped, _, err := partslim.StripStoredParts(messageWith(pdf),
		[]partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if _, err := partslim.RestoreStoredParts(stripped, func(partslim.PartRef) ([]byte, error) {
		return pdf[:len(pdf)-1], nil
	}); err == nil {
		t.Fatal("restore accepted an object of the wrong length")
	}
}

// A part whose bytes are not in the message is left alone rather than guessed
// at. This is the safety property the whole sweep rests on: locating by
// encoding means a mismatch removes nothing.
func TestStripStoredPartsSkipsAPartItCannotLocate(t *testing.T) {
	pdf := attachmentBody(5)
	raw := messageWith(pdf)
	elsewhere := attachmentBody(6)
	out, stripped, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(1, elsewhere)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if stripped != 0 {
		t.Errorf("stripped %d parts, want 0", stripped)
	}
	if !bytes.Equal(out, raw) {
		t.Errorf("a message with nothing to strip was rewritten anyway")
	}
}

// The same attachment twice cannot be told apart by its bytes, so neither copy
// is touched. Refusing is the only safe answer: splicing the first occurrence
// would leave the second behind under a stanza that claims both.
func TestStripStoredPartsSkipsAnAttachmentSentTwice(t *testing.T) {
	pdf := attachmentBody(7)
	one := messageWith(pdf)
	twice := append(append([]byte(nil), one...), one...)
	out, stripped, err := partslim.StripStoredParts(twice, []partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if stripped != 0 || !bytes.Equal(out, twice) {
		t.Errorf("stripped %d parts of an ambiguous message; want 0 and no rewrite", stripped)
	}
}

// A body the caller's digest does not describe is a caller bug, not a message
// this can reason about, so it fails loudly rather than skipping quietly.
func TestStripStoredPartsRefusesBytesThatDoNotMatchTheirDigest(t *testing.T) {
	pdf := attachmentBody(8)
	part := storedPart(1, pdf)
	part.Sha256 = partslim.Sha256HexForTest([]byte("something else"))
	if _, _, err := partslim.StripStoredParts(messageWith(pdf), []partslim.StoredPart{part}); err == nil {
		t.Fatal("strip accepted octets whose digest did not match the part it was told they were")
	}
}

// A non-UTF-8 body is the case a decoding walk could not read at all. The strip
// never decodes, so it passes through untouched — which is the whole reason
// this is done by byte offset.
func TestStripStoredPartsLeavesANonUTF8BodyIntact(t *testing.T) {
	pdf := attachmentBody(9)
	latin1 := []byte{0x63, 0x61, 0x66, 0xE9} // "café" in windows-1252
	raw := []byte("MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=windows-1252\r\n" +
		"\r\n" +
		string(latin1) + "\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"f.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		wrap76(pdf) + "\r\n" +
		"--b1--\r\n")
	out, stripped, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping a windows-1252 message: %v", err)
	}
	if stripped != 1 {
		t.Fatalf("stripped %d parts, want 1", stripped)
	}
	if !bytes.Contains(out, latin1) {
		t.Errorf("the windows-1252 body was altered; it must pass through byte for byte")
	}
}

func TestIsSlimmedRecognisesOnlyASlimmedOriginal(t *testing.T) {
	pdf := attachmentBody(10)
	raw := messageWith(pdf)
	if partslim.IsSlimmed(raw) {
		t.Error("an untouched original reads as slimmed")
	}
	stripped, _, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(1, pdf)})
	if err != nil {
		t.Fatalf("stripping: %v", err)
	}
	if !partslim.IsSlimmed(stripped) {
		t.Error("a slimmed original does not read as slimmed")
	}
}

func firstKB(b []byte) []byte {
	if len(b) > 1024 {
		return b[:1024]
	}
	return b
}

// twoAttachmentMessage carries two base64 attachments, so a strip of the
// SECOND has a neighbour it must not touch.
func twoAttachmentMessage(first, second []byte) []byte {
	return []byte("MIME-Version: 1.0\r\n" +
		"Subject: Both figures\r\n" +
		"Content-Type: multipart/mixed; boundary=\"b1\"\r\n" +
		"\r\n" +
		"--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"the visible body\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"first.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		wrap76(first) + "\r\n" +
		"--b1\r\n" +
		"Content-Type: application/pdf\r\n" +
		"Content-Disposition: attachment; filename=\"second.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		wrap76(second) + "\r\n" +
		"--b1--\r\n")
}

// A stanza names the ordinal its attachment row carries, and stripping one
// part leaves its neighbour byte-identical.
//
// The ordinal is not how the part is FOUND — location is by encoding — so this
// pins the two things the ordinal is actually for: the label a restore reads
// back, and the identity the attachment row joins on. A stanza numbered from
// the strip's own walk order rather than from the row would silently mislabel
// every message whose first attachment could not be located.
func TestStripStoredPartsNamesTheOrdinalItWasGiven(t *testing.T) {
	first, second := attachmentBody(11), attachmentBody(12)
	raw := twoAttachmentMessage(first, second)

	out, stripped, err := partslim.StripStoredParts(raw, []partslim.StoredPart{storedPart(2, second)})
	if err != nil || stripped != 1 {
		t.Fatalf("stripping the second attachment: stripped=%d err=%v", stripped, err)
	}
	if !bytes.Contains(out, []byte(partslim.PartStoredHeader+": part:2")) {
		t.Errorf("the stanza does not name part:2")
	}
	if bytes.Contains(out, []byte(partslim.PartStoredHeader+": part:1")) {
		t.Errorf("the stanza named part:1, which was never asked for")
	}
	if bytes.Contains(out, []byte(wrap76(second))) {
		t.Errorf("the second attachment's encoded body survived")
	}
	if !bytes.Contains(out, []byte(wrap76(first))) {
		t.Errorf("the FIRST attachment was collateral damage")
	}
	if !bytes.Contains(out, []byte("filename=\"first.pdf\"")) ||
		!bytes.Contains(out, []byte("filename=\"second.pdf\"")) {
		t.Errorf("a part's own header fields were lost")
	}

	restored, err := partslim.RestoreStoredParts(out, func(ref partslim.PartRef) ([]byte, error) {
		if ref.Ordinal != 2 {
			t.Errorf("restore asked for ordinal %d, want 2", ref.Ordinal)
		}
		return second, nil
	})
	if err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if !bytes.Equal(restored, raw) {
		t.Errorf("restore of a two-attachment message was not byte-exact: %d vs %d",
			len(restored), len(raw))
	}
}
