// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

func TestAStoredBodyIsAssessedOnlyWhereTheOriginalVouchesForIt(t *testing.T) {
	broken := mailOf(`multipart/mixed; boundary="A"`,
		"", "--A", "Content-Type: text/plain; charset=utf-8", "Content-Transfer-Encoding: base64",
		"", "aGFsbG8gd2VsdA==", "!!!not base64!!!", "--A--", "")
	whole := alternative("base64", "aGFsbG8=")
	headerOnly := "From: a@example.org\nTo: " + walkOwner
	longText := strings.Repeat("a long line of text ", 600)
	long := mailOf("text/plain; charset=utf-8", "", longText, "")
	longCut := textcut.BytesMarked(headerOnly+"\n\n"+strings.TrimSpace(longText), legacyMaxBodyLen, "…")
	for name, tc := range map[string]struct {
		raw    []byte
		stored string
		want   capture.BodyCompleteness
		ok     bool
	}{
		"stored as capture stores it now":     {whole, headerOnly + "\n\nhallo", capture.BodyComplete, true},
		"a broken part stored as capture now": {broken, headerOnly + "\n\nhallo welt", capture.BodyPartial, true},
		"text an older parser lost":           {broken, headerOnly, capture.BodyUnreadable, true},
		"a body somebody edited":              {whole, headerOnly + "\n\nhallo, edited", "", false},
		"a long body an older cap cut":        {long, longCut, capture.BodyPartial, true},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := AssessStoredBody(tc.raw, tc.stored)
			if got != tc.want || ok != tc.ok {
				t.Errorf("AssessStoredBody = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAShortStoredBodyIsRecoveredFromItsOriginal(t *testing.T) {
	headerOnly := "From: a@example.org\nTo: " + walkOwner
	whole := alternative("base64", "aGFsbG8=")
	longText := strings.Repeat("a long line of text ", 600)
	long := mailOf("text/plain; charset=utf-8", "", longText, "")
	longCut := textcut.BytesMarked(headerOnly+"\n\n"+strings.TrimSpace(longText), legacyMaxBodyLen, "…")
	for name, tc := range map[string]struct {
		raw          []byte
		stored, want string
		completeness capture.BodyCompleteness
		ok           bool
	}{
		"text an older parser lost": {whole, headerOnly, headerOnly + "\n\nhallo", capture.BodyComplete, true},
		"text the older cap cut":    {long, longCut, headerOnly + "\n\n" + strings.TrimSpace(longText), capture.BodyComplete, true},
		"a body somebody edited":    {whole, headerOnly + "\n\nedited", "", "", false},
		"already whole":             {whole, headerOnly + "\n\nhallo", "", "", false},
	} {
		t.Run(name, func(t *testing.T) {
			got, completeness, ok := RecoverStoredBody(tc.raw, tc.stored)
			if ok != tc.ok || (ok && (got != tc.want || completeness != tc.completeness)) {
				t.Errorf("RecoverStoredBody = %q, %q, %v; want %q, %q, %v", got, completeness, ok, tc.want, tc.completeness, tc.ok)
			}
		})
	}
}

// A body is stored with its credentials masked. A row stored before that, as it
// was written and as a repair later masked it, still reads as what it is.
func TestAStoredBodyIsAssessedInEveryShapeMaskingLeft(t *testing.T) {
	headerOnly := "From: a@example.org\nTo: " + walkOwner
	secret := mailOf("text/plain; charset=utf-8", "", "Passwort: Quokka7", "")
	longText := "Passwort: Quokka7\n" + strings.Repeat("a long line of text ", 600)
	long := mailOf("text/plain; charset=utf-8", "", longText, "")
	legacyUnmasked := textcut.BytesMarked(headerOnly+"\n\n"+strings.TrimSpace(longText), legacyMaxBodyLen, "…")
	for name, tc := range map[string]struct {
		raw    []byte
		stored string
		want   capture.BodyCompleteness
	}{
		"masked as capture stores it now":   {secret, headerOnly + "\n\nPasswort: [masked]", capture.BodyComplete},
		"stored before masking":             {secret, headerOnly + "\n\nPasswort: Quokka7", capture.BodyComplete},
		"cut by the older cap, unmasked":    {long, legacyUnmasked, capture.BodyPartial},
		"cut by the older cap, then masked": {long, credentialmask.MaskText(legacyUnmasked), capture.BodyPartial},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := AssessStoredBody(tc.raw, tc.stored)
			if got != tc.want || !ok {
				t.Errorf("AssessStoredBody = %q, %v; want %q, true", got, ok, tc.want)
			}
		})
	}

	repaired := credentialmask.MaskText(legacyUnmasked)
	body, completeness, ok := RecoverStoredBody(long, repaired)
	want := headerOnly + "\n\nPasswort: [masked]\n" + strings.TrimSpace(strings.Repeat("a long line of text ", 600))
	if !ok || body != want || completeness != capture.BodyComplete {
		t.Errorf("RecoverStoredBody of the masked short row = %.60q…, %q, %v; want the whole masked body, complete", body, completeness, ok)
	}
}

// The mask runs before the length cut. So a private key the cut would split is
// found whole, and its first half never stays on the row.
func TestAKeyTheCutWouldSplitIsStillMasked(t *testing.T) {
	line := "a long line of text\n"
	header := "From: a@example.org\nTo: " + walkOwner + "\n\n"
	filler := strings.Repeat(line, (maxBodyLen-len(header)-100)/len(line))
	key := "-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("KEYMATERIAL", 40) + "\n-----END PRIVATE KEY-----"
	msg, err := Parse(mailOf("text/plain; charset=utf-8", "", filler+key, ""), "")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if unmasked, _ := cutBody(msg.unmaskedBody(msg.body), maxBodyLen); !strings.Contains(unmasked, "BEGIN") || strings.Contains(unmasked, "END") {
		t.Fatal("the fixture's key must straddle the cut")
	}
	body, _ := msg.storedBody()
	if strings.Contains(body, "KEYMATERIAL") {
		t.Errorf("the stored body keeps the half of the key the cut left: %q", body[len(body)-80:])
	}
}

func TestACapturedSubjectIsStoredWithItsCredentialsMasked(t *testing.T) {
	raw := crlf("From: A <a@example.org>", "To: "+walkOwner, "Subject: Zugang, password: Quokka7",
		"Message-ID: <s@example.org>", "Content-Type: text/plain", "", "hallo", "")
	msg, err := Parse(raw, walkOwner)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	fields, ok := msg.ToRecord("gmail", raw).Fields.(capture.ActivityFields)
	if !ok || fields.Subject != "Zugang, password: [masked]" {
		t.Errorf("subject = %q, want the password masked behind its label", fields.Subject)
	}
}

// The record says masking changed it only when a credential was masked, never
// for text a sender typed as "[masked]".
func TestTheRecordSaysWhenMaskingChangedIt(t *testing.T) {
	for name, tc := range map[string]struct {
		text string
		want bool
	}{
		"a password":      {"Passwort: Quokka7", true},
		"a typed marker":  {"hello [masked]", false},
		"nothing to mask": {"hello", false},
	} {
		t.Run(name, func(t *testing.T) {
			raw := mailOf("text/plain; charset=utf-8", "", tc.text, "")
			msg, err := Parse(raw, walkOwner)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			fields, ok := msg.ToRecord("gmail", raw).Fields.(capture.ActivityFields)
			if !ok || fields.CredentialsMasked != tc.want {
				t.Errorf("CredentialsMasked = %v, want %v", fields.CredentialsMasked, tc.want)
			}
		})
	}
}
