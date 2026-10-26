// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Mail shapes whose text go-message does not hand over on its own. Each case
// goes through Parse and ToRecord, the path every connector takes.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/htmltext"
)

const walkOwner = "m@example.com"

// mailOf wraps a MIME body under one top-level Content-Type.
func mailOf(contentType string, body ...string) []byte {
	lines := []string{
		"From: A <a@example.org>",
		"To: " + walkOwner,
		"Subject: AW: test",
		"Message-ID: <x@example.org>",
		"MIME-Version: 1.0",
		"Content-Type: " + contentType,
	}
	return crlf(append(lines, body...)...)
}

// alternative is a multipart/alternative of a plain part with the given
// transfer encoding and body, then an HTML part.
func alternative(encoding, plain string) []byte {
	return mailOf(`multipart/alternative; boundary="A"`,
		"",
		"--A",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: "+encoding,
		"",
		plain,
		"--A",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>hallo html</p>",
		"--A--",
		"",
	)
}

func parseFields(t *testing.T, raw []byte) (Message, capture.ActivityFields) {
	t.Helper()
	msg, err := Parse(raw, walkOwner)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("imap", raw).Fields.(capture.ActivityFields)
	if !ok {
		t.Fatal("ToRecord did not produce ActivityFields")
	}
	return msg, fields
}

func TestParseReadsTextPastAQuotedTransferEncoding(t *testing.T) {
	cases := map[string]struct {
		encoding, plain, want string
		completeness          capture.BodyCompleteness
	}{
		"quoted base64":             {`"base64"`, "aGFsbG8=", "hallo", capture.BodyComplete},
		"blank plain falls to html": {`"base64"`, "", "hallo html", capture.BodyComplete},
		"trailing semicolon":        {"8bit;", "hallo", "hallo", capture.BodyComplete},
		"hyphenated bit width":      {"7-bit", "hallo", "hallo", capture.BodyComplete},
		// Kept for a reader, and not taken as the mail's whole text.
		"unknown value keeps raw": {"x-unknown", "aGFsbG8=", "aGFsbG8=", capture.BodyPartial},
		"quoted quoted-printable": {`"quoted-printable"`, "hal=6Co", "hallo", capture.BodyComplete},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			msg, fields := parseFields(t, alternative(tc.encoding, tc.plain))
			if msg.body != tc.want {
				t.Errorf("body = %q, want %q", msg.body, tc.want)
			}
			if fields.BodyCompleteness != tc.completeness {
				t.Errorf("BodyCompleteness = %q, want %q", fields.BodyCompleteness, tc.completeness)
			}
		})
	}
}

// go-message reports only the charset error when a part has both a bad
// encoding and an unknown charset. So the decision to decode reads the header.
func TestParseDecodesABadEncodingBesideAnUnknownCharset(t *testing.T) {
	raw := mailOf(`multipart/mixed; boundary="A"`,
		"",
		"--A",
		"Content-Type: text/plain; charset=x-unknown",
		`Content-Transfer-Encoding: "base64"`,
		"",
		"aGFsbG8=",
		"--A--",
		"",
	)
	msg, _ := parseFields(t, raw)
	if msg.body != "hallo" {
		t.Errorf("body = %q, want the decoded %q", msg.body, "hallo")
	}
}

func TestParseKeepsFilesAfterAnUndecodablePart(t *testing.T) {
	raw := mailOf(`multipart/mixed; boundary="A"`,
		"",
		"--A",
		"Content-Type: text/plain; charset=utf-8",
		`Content-Transfer-Encoding: "base64"`,
		"",
		"aGFsbG8=",
		"--A",
		"Content-Type: application/pdf",
		`Content-Disposition: attachment; filename="offer.pdf"`,
		"",
		"%PDF-1.4 offer",
		"--A--",
		"",
	)
	msg, _ := parseFields(t, raw)
	if len(msg.parts) != 1 || msg.parts[0].Filename != "offer.pdf" {
		t.Fatalf("parts = %v, want offer.pdf", msg.parts)
	}
	for _, drop := range msg.partDrops {
		if drop.Reason == DropWalkTruncated {
			t.Errorf("drops = %v, want no truncated walk", msg.partDrops)
		}
	}
}

func TestParseReadsATopLevelPartWithAnUnknownEncoding(t *testing.T) {
	raw := mailOf("text/plain; charset=utf-8",
		`Content-Transfer-Encoding: "quoted-printable"`,
		"",
		"Gr=C3=BC=C3=9Fe",
		"",
	)
	msg, _ := parseFields(t, raw)
	if msg.body != "Grüße" {
		t.Errorf("body = %q, want %q", msg.body, "Grüße")
	}
}

func TestParseKeepsWhatDecodedBeforeABrokenBase64(t *testing.T) {
	raw := mailOf(`multipart/mixed; boundary="A"`,
		"",
		"--A",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: base64",
		"",
		"aGFsbG8gd2VsdA==",
		"!!!not base64!!!",
		"--A",
		"Content-Type: application/pdf",
		`Content-Disposition: attachment; filename="offer.pdf"`,
		"",
		"%PDF-1.4 offer",
		"--A--",
		"",
	)
	msg, fields := parseFields(t, raw)
	if !strings.HasPrefix(msg.body, "hallo") {
		t.Errorf("body = %q, want the decoded start %q", msg.body, "hallo welt")
	}
	if len(msg.parts) != 1 {
		t.Errorf("parts = %v, want the PDF after the broken part", msg.parts)
	}
	// The start is shown, and the mail says the rest is missing.
	if fields.BodyCompleteness != capture.BodyPartial {
		t.Errorf("BodyCompleteness = %q, want %q", fields.BodyCompleteness, capture.BodyPartial)
	}
}

// forwarded wraps an original as a message/rfc822 part, with the disposition
// given, beside a blank plain part.
func forwarded(disposition string, original ...string) []byte {
	body := []string{
		"",
		"--F",
		"Content-Type: text/plain; charset=utf-8",
		"",
		" ",
		"--F",
		"Content-Type: message/rfc822",
	}
	if disposition != "" {
		body = append(body, "Content-Disposition: "+disposition)
	}
	body = append(body, "")
	body = append(body, original...)
	body = append(body, "--F--", "")
	return mailOf(`multipart/mixed; boundary="F"`, body...)
}

func originalWith(text string) []string {
	return []string{
		"From: B <b@example.org>",
		"Subject: test",
		"Content-Type: text/plain; charset=utf-8",
		"",
		text,
	}
}

func TestParseReadsTheTextOfAForwardedOriginal(t *testing.T) {
	cases := map[string][]byte{
		"as an attachment":          forwarded(`attachment; filename="original.eml"`, originalWith("the original answer")...),
		"as an unnamed inline part": forwarded("inline", originalWith("the original answer")...),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			msg, fields := parseFields(t, raw)
			if msg.body != "the original answer" {
				t.Errorf("body = %q, want the original's text", msg.body)
			}
			// The original's words are shown, and never read as the sender's.
			if fields.BodyCompleteness != capture.BodyForwarded {
				t.Errorf("BodyCompleteness = %q, want %q", fields.BodyCompleteness, capture.BodyForwarded)
			}
		})
	}
	t.Run("one level deep", func(t *testing.T) {
		inner := forwarded("inline", originalWith("two levels down")...)
		msg, _ := parseFields(t, forwarded("inline", strings.Split(string(inner), "\r\n")...))
		if msg.body != "" {
			t.Errorf("body = %q, want the doubly nested original left unopened", msg.body)
		}
	})
}

func TestParseMarksAnOpaqueBodyUnreadable(t *testing.T) {
	cases := map[string][]byte{
		"winmail.dat only": mailOf(`multipart/mixed; boundary="A"`,
			"", "--A",
			"Content-Type: application/ms-tnef",
			`Content-Disposition: attachment; filename="winmail.dat"`,
			"", "eJ8+IgAAAQ==", "--A--", ""),
		"enveloped s/mime": mailOf(`application/pkcs7-mime; smime-type=enveloped-data; name="smime.p7m"`,
			"Content-Transfer-Encoding: base64",
			"", "MIAGCSqGSIb3DQEHA6CAMIACAQAx", ""),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			msg, fields := parseFields(t, raw)
			if msg.body != "" {
				t.Fatalf("body = %q, want none", msg.body)
			}
			if fields.BodyCompleteness != capture.BodyUnreadable {
				t.Errorf("BodyCompleteness = %q for a body only a mail program can open", fields.BodyCompleteness)
			}
		})
	}
}

func TestParseCallsReadableTextComplete(t *testing.T) {
	cases := map[string][]byte{
		"text beside winmail.dat": mailOf(`multipart/mixed; boundary="A"`,
			"", "--A", "Content-Type: text/plain", "", "see you", "--A",
			"Content-Type: application/ms-tnef",
			`Content-Disposition: attachment; filename="winmail.dat"`,
			"", "eJ8+IgAAAQ==", "--A--", ""),
		"a pdf only": mailOf(`multipart/mixed; boundary="A"`,
			"", "--A", "Content-Type: application/pdf",
			`Content-Disposition: attachment; filename="offer.pdf"`,
			"", "%PDF-1.4 offer", "--A--", ""),
		"signed-data with no text": mailOf(`application/pkcs7-mime; smime-type=signed-data; name="smime.p7m"`,
			"", "MIAGCSqGSIb3DQEHAqCAMIACAQEx", ""),
		"s/mime file as an attachment": mailOf(`multipart/mixed; boundary="A"`,
			"", "--A", "Content-Type: application/pkcs7-mime",
			`Content-Disposition: attachment; filename="cert.p7m"`,
			"", "MIAGCSqGSIb3DQEHA6CA", "--A--", ""),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, fields := parseFields(t, raw)
			if fields.BodyCompleteness != capture.BodyComplete {
				t.Errorf("BodyCompleteness = %q; body = %q", fields.BodyCompleteness, fields.Body)
			}
		})
	}
}

// Any text part not read in full makes the text partial, even when another
// text part was read. Nothing proves the lost part said the same.
func TestParseCallsTextPartialWhenAnyTextPartWasLost(t *testing.T) {
	cases := map[string][]byte{
		"undecodable plain then html": alternative("x-unknown", ""),
		"broken at byte 0, then readable": mailOf(`multipart/mixed; boundary="A"`,
			"", "--A", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64",
			"", "!!!not base64!!!", "--A", "Content-Type: text/plain", "", "harmless", "--A--", ""),
		"html only, broken halfway": mailOf("text/html; charset=utf-8",
			"Content-Transfer-Encoding: base64", "", "PHA+aGFsbG88L3A+", "!!!not base64!!!", ""),
		"cut at the per-part limit": mailOf("text/plain; charset=utf-8",
			"", strings.Repeat("x", maxPartBytes+10), ""),
		"broken outer part beside a forward": mailOf(`multipart/mixed; boundary="F"`,
			"", "--F", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64",
			"", "!!!not base64!!!", "--F", "Content-Type: message/rfc822", "",
			strings.Join(originalWith("we will refund you"), "\r\n"), "--F--", ""),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, fields := parseFields(t, raw)
			if fields.BodyCompleteness == capture.BodyComplete {
				t.Errorf("BodyCompleteness = complete for text a part of which was lost")
			}
		})
	}
}

// A forwarded original is walked under the same bound as the mail itself.
func TestParseBoundsTheWalkInsideAForwardedOriginal(t *testing.T) {
	inner := []string{"From: B <b@example.org>", `Content-Type: multipart/mixed; boundary="I"`, ""}
	for range maxPartsExamined + 5 {
		inner = append(inner, "--I", "Content-Type: application/octet-stream", "", "x")
	}
	inner = append(inner, "--I", "Content-Type: text/plain", "", "past the bound", "--I--")
	msg, fields := parseFields(t, forwarded("inline", inner...))
	if msg.body != "" || fields.BodyCompleteness == capture.BodyComplete {
		t.Errorf("body = %q (%q), want the walk stopped before the text past the bound", msg.body, fields.BodyCompleteness)
	}
}

// The bound counts every part looked at, not only the files admitted: unnamed
// text parts and nested containers cost the same walk.
func TestParseBoundsTheWalkOnUnnamedParts(t *testing.T) {
	body := []string{""}
	for range maxPartsExamined {
		body = append(body, "--A", `Content-Type: multipart/mixed; boundary="B"`, "",
			"--B", "Content-Type: text/plain", "", "x", "--B--")
	}
	body = append(body, "--A--", "")
	msg, _ := parseFields(t, mailOf(`multipart/mixed; boundary="A"`, body...))
	var truncated bool
	for _, drop := range msg.partDrops {
		truncated = truncated || drop.Reason == DropWalkTruncated
	}
	if !truncated {
		t.Errorf("drops = %v, want the walk to stop at %d parts and say so", msg.partDrops, maxPartsExamined)
	}
	// The text read before the bound is kept, and it is not the whole mail.
	if got := msg.bodyCompleteness; msg.body == "" || got != capture.BodyPartial {
		t.Errorf("body = %q with completeness %q, want the read prefix as partial", msg.body, got)
	}
}

// A walk that ends early holds text it can show, never text it can vouch for.
func TestParseCallsTextBeforeABrokenStructurePartial(t *testing.T) {
	raw := mailOf(`multipart/mixed; boundary="A"`,
		"", "--A", "Content-Type: text/plain", "", "We will end the contract.",
		"--A", "Content-Type: text/plain", "", "Unless you renew by Friday.")
	msg, fields := parseFields(t, raw)
	if !strings.Contains(msg.body, "We will end the contract.") {
		t.Fatalf("body = %q, want the text read before the break", msg.body)
	}
	if fields.BodyCompleteness != capture.BodyPartial {
		t.Errorf("BodyCompleteness = %q, want partial", fields.BodyCompleteness)
	}
}

// What capture stores tells the truth about what it left out. An ordinary long
// mail is stored whole; only a body past the storage cap, or an HTML render past
// its cap, is partial.
func TestTheStoredBodySaysWhetherItIsWhole(t *testing.T) {
	cases := map[string]struct {
		raw  []byte
		want capture.BodyCompleteness
	}{
		"a long mail stored whole": {mailOf("text/plain; charset=utf-8", "", strings.Repeat("a long line of text ", 600), ""), capture.BodyComplete},
		"past the storage cap":     {mailOf("text/plain; charset=utf-8", "", strings.Repeat("x", maxBodyLen+10), ""), capture.BodyPartial},
		"an HTML render past its cap": {mailOf("text/html; charset=utf-8", "",
			strings.Repeat("<p>word word word</p>", htmltext.MaxRenderedLen/10), ""), capture.BodyPartial},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, fields := parseFields(t, tc.raw)
			if fields.BodyCompleteness != tc.want {
				t.Errorf("BodyCompleteness = %q, want %q", fields.BodyCompleteness, tc.want)
			}
		})
	}
}
