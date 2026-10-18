// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

// German mail clients still send headers and bodies in Latin-1 or
// Windows-1252. Each must reach the stored record as UTF-8. An umlaut
// replaced there cannot be recovered from the record.
func TestParseDecodesLegacyCharsetsToUTF8(t *testing.T) {
	cases := map[string]struct {
		from, subject, contentType, body string
	}{
		"iso-8859-1": {
			from:        "=?iso-8859-1?q?Dirk_H=F6rig?= <dirk@acme.de>",
			subject:     "=?iso-8859-1?q?Angebot_f=FCr_H=F6rig?=",
			contentType: "text/plain; charset=iso-8859-1",
			body:        "Gr=FC=DFe, Dirk H=F6rig",
		},
		"windows-1252": {
			from:        "=?windows-1252?q?Dirk_H=F6rig?= <dirk@acme.de>",
			subject:     "=?windows-1252?q?Angebot_f=FCr_H=F6rig_=96_neu?=",
			contentType: "text/plain; charset=windows-1252",
			body:        "Gr=FC=DFe, Dirk H=F6rig",
		},
		"windows-1252 html only": {
			from:        "=?windows-1252?q?Dirk_H=F6rig?= <dirk@acme.de>",
			subject:     "=?windows-1252?q?Angebot_f=FCr_H=F6rig?=",
			contentType: "text/html; charset=windows-1252",
			body:        "<p>Gr=FC=DFe, Dirk H=F6rig</p>",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			raw := crlf(
				"From: "+tc.from,
				"To: me@myco.com",
				"Subject: "+tc.subject,
				"Date: Wed, 04 Jun 2026 08:00:00 +0000",
				"Message-ID: <latin1@acme.de>",
				"Content-Type: "+tc.contentType,
				"Content-Transfer-Encoding: quoted-printable",
				"",
				tc.body,
				"",
			)
			msg, err := Parse(raw, "me@myco.com")
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			rec := msg.ToRecord("imap", raw)
			if got := rec.Counterparty.DisplayName; got != "Dirk Hörig" {
				t.Errorf("counterparty name = %q, want %q", got, "Dirk Hörig")
			}
			fields, ok := rec.Fields.(capture.ActivityFields)
			if !ok {
				t.Fatalf("Fields is %T, want capture.ActivityFields", rec.Fields)
			}
			if !strings.HasPrefix(fields.Subject, "Angebot für Hörig") {
				t.Errorf("subject = %q, want it to start with %q", fields.Subject, "Angebot für Hörig")
			}
			if !strings.Contains(fields.Body, "Grüße, Dirk Hörig") {
				t.Errorf("body = %q, want it to carry %q", fields.Body, "Grüße, Dirk Hörig")
			}
		})
	}
}

// A file is stored as the sender sent it, so a restore hands back the same
// bytes. Decoding a Latin-1 text attachment to UTF-8 would change them, so
// only the body text read for the record is decoded.
func TestParseKeepsALegacyCharsetAttachmentByteIdentical(t *testing.T) {
	attachment := "Gr\xfc\xdfe, Dirk H\xf6rig\r\n"
	raw := crlf(
		"From: =?windows-1252?q?Dirk_H=F6rig?= <dirk@acme.de>",
		"To: me@myco.com",
		"Subject: =?iso-8859-1?q?Angebot_f=FCr_H=F6rig?=",
		"Date: Wed, 04 Jun 2026 08:00:00 +0000",
		"Message-ID: <latin1-attachment@acme.de>",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=iso-8859-1",
		"Content-Transfer-Encoding: 8bit",
		"",
		"Gr\xfc\xdfe, Dirk H\xf6rig",
		"--b1",
		"Content-Type: text/plain; charset=iso-8859-1",
		`Content-Disposition: attachment; filename="notiz.txt"`,
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(attachment)),
		"--b1--",
		"",
	)
	msg, err := Parse(raw, "me@myco.com")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rec := msg.ToRecord("imap", raw)
	if len(rec.Parts) != 1 {
		t.Fatalf("parts = %d, want the one attachment", len(rec.Parts))
	}
	if got := string(rec.Parts[0].Body); got != attachment {
		t.Errorf("attachment bytes = %q, want them unchanged as %q", got, attachment)
	}
	if got := rec.Counterparty.DisplayName; got != "Dirk Hörig" {
		t.Errorf("counterparty name = %q, want %q", got, "Dirk Hörig")
	}
	fields, ok := rec.Fields.(capture.ActivityFields)
	if !ok {
		t.Fatalf("Fields is %T, want capture.ActivityFields", rec.Fields)
	}
	if fields.Subject != "Angebot für Hörig" {
		t.Errorf("subject = %q, want %q", fields.Subject, "Angebot für Hörig")
	}
	if !strings.Contains(fields.Body, "Grüße, Dirk Hörig") {
		t.Errorf("body = %q, want it to carry %q", fields.Body, "Grüße, Dirk Hörig")
	}
}
