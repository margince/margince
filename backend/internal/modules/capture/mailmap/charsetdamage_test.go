// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"testing"
)

func latin1Mail(subject string) []byte {
	return crlf(
		"From: A <a@example.org>", "To: "+walkOwner, "Subject: "+subject, "Message-ID: <c@example.org>",
		"MIME-Version: 1.0", "Content-Type: text/plain; charset=windows-1252",
		"Content-Transfer-Encoding: quoted-printable", "", "Mit freundlichen Gr=FC=DFen", "")
}

func TestACharsetDamagedMailIsRecoveredOnlyFromTheShapeTheOlderParserStored(t *testing.T) {
	const encoded = "=?Windows-1252?Q?Angebot_f=FCr_Sie?="
	header := "From: a@example.org\nTo: " + walkOwner + "\n\n"
	raw := latin1Mail(encoded)
	for name, tc := range map[string]struct {
		stored, body string
		wantSubject  string
		wantBody     bool
		wantOK       bool
	}{
		"both damaged":             {encoded, header + "Mit freundlichen Gr??en", "Angebot für Sie", true, true},
		"replacement characters":   {encoded, header + "Mit freundlichen Gr��en", "Angebot für Sie", true, true},
		"subject only":             {encoded, header + "Mit freundlichen Grüßen", "Angebot für Sie", false, true},
		"body only":                {"Angebot für Sie", header + "Mit freundlichen Gr??en", "", true, true},
		"a subject somebody typed": {"Mein Angebot", header + "Mit freundlichen Grüßen", "", false, false},
		"a body somebody edited":   {"Angebot für Sie", header + "Mit freundlichen Gr??en!", "", false, false},
		"already right":            {"Angebot für Sie", header + "Mit freundlichen Grüßen", "", false, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := RecoverCharsetDamage(raw, tc.stored, tc.body)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if tc.wantSubject != "" && (got.Subject == nil || *got.Subject != tc.wantSubject) {
				t.Errorf("subject = %v, want %q", got.Subject, tc.wantSubject)
			}
			if tc.wantSubject == "" && got.Subject != nil {
				t.Errorf("subject = %q, want it left alone", *got.Subject)
			}
			if got.BodyChanged != tc.wantBody {
				t.Fatalf("body changed = %v, want %v", got.BodyChanged, tc.wantBody)
			}
			if tc.wantBody && got.Body != header+"Mit freundlichen Grüßen" {
				t.Errorf("body = %q", got.Body)
			}
		})
	}
}
