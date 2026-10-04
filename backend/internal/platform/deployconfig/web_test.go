// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"os"
	"strings"
	"testing"
	"time"
)

// The expiry is unquoted on purpose: YAML reads that spelling as a timestamp,
// and an operator writing it the natural way must still get a working file.
const securityTxtConfig = `version: 1
web:
  security_txt:
    contact:
      - mailto:security@example.org
      - https://example.org/report
    expires: 2030-01-01T00:00:00+02:00
    policy: https://example.org/disclosure
    preferred_languages: [en, de-CH]
`

func TestSecurityTxtRendersEveryConfiguredFieldInRFC9116Form(t *testing.T) {
	cfg, err := Parse([]byte(securityTxtConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := "Contact: mailto:security@example.org\n" +
		"Contact: https://example.org/report\n" +
		"Expires: 2029-12-31T22:00:00Z\n" +
		"Policy: https://example.org/disclosure\n" +
		"Preferred-Languages: en, de-CH\n"
	if got := cfg.Web.SecurityTxtBody(); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestSecurityTxtOmitsTheOptionalFieldsItWasNotGiven(t *testing.T) {
	cfg, err := Parse([]byte("version: 1\nweb: { security_txt: { contact: [\"tel:+1-201-555-0123\"], expires: \"2030-01-01T00:00:00Z\" } }\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := cfg.Web.SecurityTxtBody(), "Contact: tel:+1-201-555-0123\nExpires: 2030-01-01T00:00:00Z\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestAnUnsetSecurityTxtRendersNothing(t *testing.T) {
	cfg, err := Parse([]byte("version: 1\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if body := cfg.Web.SecurityTxtBody(); body != "" {
		t.Fatalf("an absent block rendered %q; the route must stay unmounted", body)
	}
	if w := cfg.Web.Warnings(time.Now()); w != nil {
		t.Fatalf("an absent block warned: %v", w)
	}
}

func TestSecurityTxtRefusesWhatRFC9116OrTheFileFormatCannotCarry(t *testing.T) {
	const expires = `expires: "2030-01-01T00:00:00Z"`
	cases := map[string]string{
		"no contact":              `{ ` + expires + ` }`,
		"no expires":              `{ contact: ["mailto:a@example.org"] }`,
		"expires not rfc3339":     `{ contact: ["mailto:a@example.org"], expires: "next year" }`,
		"bare address":            `{ contact: ["a@example.org"], ` + expires + ` }`,
		"plain http contact":      `{ contact: ["http://example.org"], ` + expires + ` }`,
		"https without a host":    `{ contact: ["https:///report"], ` + expires + ` }`,
		"empty mailto":            `{ contact: ["mailto:"], ` + expires + ` }`,
		"line break injected":     `{ contact: ["mailto:a@example.org\nPolicy: https://evil.example"], ` + expires + ` }`,
		"space in contact":        `{ contact: ["mailto:a@example.org x"], ` + expires + ` }`,
		"mailto policy":           `{ contact: ["mailto:a@example.org"], policy: "mailto:a@example.org", ` + expires + ` }`,
		"language list joined":    `{ contact: ["mailto:a@example.org"], preferred_languages: ["en, de"], ` + expires + ` }`,
		"mailto with no mailbox":  `{ contact: ["mailto:?subject=report"], ` + expires + ` }`,
		"mailto not an address":   `{ contact: ["mailto:security"], ` + expires + ` }`,
		"expires comma fraction":  `{ contact: ["mailto:a@example.org"], expires: "2030-01-01T00:00:00,5Z" }`,
		"language bare singleton": `{ contact: ["mailto:a@example.org"], preferred_languages: ["en-a"], ` + expires + ` }`,
		"language underscore":     `{ contact: ["mailto:a@example.org"], preferred_languages: ["en_US"], ` + expires + ` }`,
	}
	for name, block := range cases {
		_, err := Parse([]byte("version: 1\nweb: { security_txt: " + block + " }\n"))
		if err == nil {
			t.Errorf("%s: parsed without error", name)
			continue
		}
		if !strings.Contains(err.Error(), "web.security_txt") {
			t.Errorf("%s: the error does not name the key: %v", name, err)
		}
	}
}

func TestSecurityTxtWarnsAtBootWhenItsExpiryIsPastOrTooFarOut(t *testing.T) {
	cfg, err := Parse([]byte(securityTxtConfig))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	expiry := time.Date(2029, 12, 31, 22, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		now  time.Time
		want string
	}{
		"expired":            {expiry.Add(time.Hour), "not in the future"},
		"expiring right now": {expiry, "not in the future"},
		"over a year away":   {expiry.AddDate(-2, 0, 0), "more than a year away"},
		"inside the year":    {expiry.AddDate(0, -6, 0), ""},
	}
	for name, tc := range cases {
		got := strings.Join(cfg.Web.Warnings(tc.now), " ")
		if tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
			t.Errorf("%s: warnings = %q, want one saying %q", name, got, tc.want)
		}
	}
}

func TestAnUnparseableExpiryIsLeftToParseNotWarnedAbout(t *testing.T) {
	if w := (Web{SecurityTxt: &SecurityTxt{Expires: "soon"}}).Warnings(time.Now()); w != nil {
		t.Fatalf("warned about an expiry Parse refuses: %v", w)
	}
}

// The block the example invites an operator to uncomment is held to the parser.
func TestTheDocumentedSecurityTxtBlockParses(t *testing.T) {
	raw, err := os.ReadFile("../../../../config/margince.example.yaml")
	if err != nil {
		t.Fatalf("reading the shipped example: %v", err)
	}
	block := commentedBlock(t, string(raw), "web:", "security_txt:")
	cfg, err := Parse([]byte("version: 1\n" + block))
	if err != nil {
		t.Fatalf("the documented web.security_txt block does not parse: %v\n%s", err, block)
	}
	if !strings.Contains(cfg.Web.SecurityTxtBody(), "Policy: ") {
		t.Fatalf("the documented block rendered without its optional fields:\n%s", cfg.Web.SecurityTxtBody())
	}
}
