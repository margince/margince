// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// The expiry is unquoted on purpose: YAML reads that spelling as a timestamp,
// and an operator writing it the natural way must still get a working file.
const securityTxtConfig = `web:
  security_txt:
    contact:
      - mailto:security@example.org
      - https://example.org/report
    expires: 2030-01-01T00:00:00+02:00
    policy: https://example.org/disclosure
    preferred_languages: [en, de-CH]
`

func loadSecurityTxt(t *testing.T, body string) (*SecurityTxt, error) {
	t.Helper()
	cfg, err := configtest.Load(t, "version: 1\n"+body, nil, nil, WebSecurityTxt)
	if err != nil {
		return nil, err
	}
	return config.Val(cfg, WebSecurityTxt), nil
}

func mustLoadSecurityTxt(t *testing.T, body string) *SecurityTxt {
	t.Helper()
	s, err := loadSecurityTxt(t, body)
	if err != nil {
		t.Fatalf("loading %q: %v", body, err)
	}
	return s
}

func TestSecurityTxtRendersEveryConfiguredFieldInRFC9116Form(t *testing.T) {
	want := "Contact: mailto:security@example.org\n" +
		"Contact: https://example.org/report\n" +
		"Expires: 2029-12-31T22:00:00Z\n" +
		"Policy: https://example.org/disclosure\n" +
		"Preferred-Languages: en, de-CH\n"
	if got := securityTxtBody(mustLoadSecurityTxt(t, securityTxtConfig)); got != want {
		t.Fatalf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestSecurityTxtOmitsTheOptionalFieldsItWasNotGiven(t *testing.T) {
	s := mustLoadSecurityTxt(t, "web: { security_txt: { contact: [\"tel:+1-201-555-0123\"], expires: \"2030-01-01T00:00:00Z\" } }\n")
	if got, want := securityTxtBody(s), "Contact: tel:+1-201-555-0123\nExpires: 2030-01-01T00:00:00Z\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestAnUnsetSecurityTxtRendersNothing(t *testing.T) {
	s := mustLoadSecurityTxt(t, "")
	if body := securityTxtBody(s); body != "" {
		t.Fatalf("an absent block rendered %q; the route must stay unmounted", body)
	}
	if w := securityTxtWarnings(s, clocktest.Now(t)); w != nil {
		t.Fatalf("an absent block warned: %v", w)
	}
}

func TestSecurityTxtRefusesWhatRFC9116OrTheFileFormatCannotCarry(t *testing.T) {
	const expires = `expires: "2030-01-01T00:00:00Z"`
	cases := map[string]string{
		"empty block":             `{}`,
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
		"unknown field":           `{ contact: ["mailto:a@example.org"], polcy: "https://x.example", ` + expires + ` }`,
	}
	for name, block := range cases {
		_, err := loadSecurityTxt(t, "web: { security_txt: "+block+" }\n")
		if err == nil {
			t.Errorf("%s: loaded without error", name)
			continue
		}
		if !strings.Contains(err.Error(), "web.security_txt") {
			t.Errorf("%s: the error does not name the key: %v", name, err)
		}
	}
}

func TestSecurityTxtWarnsAtBootWhenItsExpiryIsPastOrTooFarOut(t *testing.T) {
	s := mustLoadSecurityTxt(t, securityTxtConfig)
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
		got := strings.Join(securityTxtWarnings(s, tc.now), " ")
		if tc.want == "" && got != "" || !strings.Contains(got, tc.want) {
			t.Errorf("%s: warnings = %q, want one saying %q", name, got, tc.want)
		}
	}
}

func TestAnUnparseableExpiryIsLeftToTheLoaderNotWarnedAbout(t *testing.T) {
	if w := securityTxtWarnings(&SecurityTxt{Expires: "soon"}, clocktest.Now(t)); w != nil {
		t.Fatalf("warned about an expiry the loader refuses: %v", w)
	}
}

// The block the example invites an operator to uncomment is held to the loader.
func TestTheDocumentedSecurityTxtBlockLoads(t *testing.T) {
	raw, err := os.ReadFile("../../../config/margince.example.yaml")
	if err != nil {
		t.Fatalf("reading the shipped example: %v", err)
	}
	block := uncommentedBlock(t, string(raw), "web:", "security_txt:")
	if body := securityTxtBody(mustLoadSecurityTxt(t, block)); !strings.Contains(body, "Policy: ") {
		t.Fatalf("the documented block rendered without its optional fields:\n%s", body)
	}
}

// uncommentedBlock returns the commented-out block of the example that starts
// with `# start` and contains want, with its comment marks removed.
func uncommentedBlock(t *testing.T, doc, start, want string) string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "# "+start {
			continue
		}
		var out []string
		for _, l := range lines[i:] {
			trimmed := strings.TrimSpace(l)
			if trimmed == "" || !strings.HasPrefix(trimmed, "#") {
				break
			}
			out = append(out, strings.TrimPrefix(strings.TrimSuffix(l, "#"), "# "))
		}
		if block := strings.Join(out, "\n") + "\n"; strings.Contains(block, want) {
			return block
		}
	}
	t.Fatalf("the example no longer documents a commented %s block containing %q", start, want)
	return ""
}
