// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

// The web block of margince.yaml: what this installation publishes to the
// browsers and tools that reach its origin, beyond the app itself. Today that is
// one document, the RFC 9116 security.txt.
//
// The contact belongs to whoever OPERATES the installation, not to the product:
// a researcher who finds a hole in one deployment reports it to whoever runs
// it. So no default exists and nothing is compiled in. With the block absent
// the route is not mounted and answers 404.

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Web is the operator's published-documents section.
type Web struct {
	// SecurityTxt is served at /.well-known/security.txt when set.
	SecurityTxt *SecurityTxt `yaml:"security_txt"`
}

// SecurityTxt is the RFC 9116 contact file, field by field.
type SecurityTxt struct {
	// Contact is one or more URIs a finder reports to: mailto:, https: or tel:.
	Contact []string `yaml:"contact"`
	// Expires is the RFC 3339 instant after which a reader treats the file as stale.
	Expires string `yaml:"expires"`
	// Policy is an optional https: link to the disclosure policy.
	Policy string `yaml:"policy"`
	// PreferredLanguages is an optional list of language tags (en, de) a report may be written in.
	PreferredLanguages []string `yaml:"preferred_languages"`
}

// languageTag is the BCP 47 shape at the precision a header line needs: letters
// first, then hyphen-joined alphanumeric subtags. It refuses the comma and the
// line break a value would otherwise smuggle into the rendered file.
var languageTag = regexp.MustCompile(`^[A-Za-z]{1,8}(-[A-Za-z0-9]{1,8})*$`)

// securityTxtHorizon is RFC 9116's advice for how far ahead Expires should sit:
// less than a year, so the file is looked at again.
const securityTxtHorizon = 365 * 24 * time.Hour

func (w Web) validate() error {
	if w.SecurityTxt == nil {
		return nil
	}
	return w.SecurityTxt.validate()
}

func (s SecurityTxt) validate() error {
	if len(s.Contact) == 0 {
		return errors.New("deployconfig: web.security_txt.contact needs at least one mailto:, https: or tel: URI")
	}
	for _, c := range s.Contact {
		if err := checkURI(c, "mailto", "https", "tel"); err != nil {
			return fmt.Errorf("deployconfig: web.security_txt.contact %q: %w", c, err)
		}
	}
	if _, err := s.expiry(); err != nil {
		return err
	}
	if s.Policy != "" {
		if err := checkURI(s.Policy, "https"); err != nil {
			return fmt.Errorf("deployconfig: web.security_txt.policy %q: %w", s.Policy, err)
		}
	}
	for _, lang := range s.PreferredLanguages {
		if !languageTag.MatchString(lang) {
			return fmt.Errorf("deployconfig: web.security_txt.preferred_languages %q is not a language tag such as en or de-CH", lang)
		}
	}
	return nil
}

// expiry parses Expires; RFC 9116 makes the field mandatory, so absent is an error.
func (s SecurityTxt) expiry() (time.Time, error) {
	if s.Expires == "" {
		return time.Time{}, errors.New("deployconfig: web.security_txt.expires is required (RFC 3339, e.g. 2030-01-01T00:00:00Z)")
	}
	at, err := time.Parse(time.RFC3339, s.Expires)
	if err != nil {
		return time.Time{}, fmt.Errorf("deployconfig: web.security_txt.expires %q is not an RFC 3339 date-time such as 2030-01-01T00:00:00Z", s.Expires)
	}
	return at, nil
}

// checkURI holds a value to an absolute URI in one of schemes, with a host for
// https. url.Parse already refuses control characters, which is what keeps a
// value from adding a line of its own to the rendered file; whitespace is
// refused here for the same reason.
func checkURI(raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || strings.ContainsAny(raw, " \t") {
		return errors.New("is not a URI")
	}
	for _, scheme := range schemes {
		if u.Scheme != scheme {
			continue
		}
		if scheme == "https" && u.Host == "" {
			return errors.New("names no host")
		}
		if scheme != "https" && u.Opaque == "" {
			return errors.New("names no address")
		}
		return nil
	}
	return fmt.Errorf("is not a %s: URI", strings.Join(schemes, ":, "))
}

// SecurityTxtBody renders the file the route serves, or "" when the block is
// absent — the composition layer reads the empty string as "do not mount".
// It assumes a Config that Parse accepted.
func (w Web) SecurityTxtBody() string {
	s := w.SecurityTxt
	if s == nil {
		return ""
	}
	at, _ := s.expiry()
	var b strings.Builder
	for _, c := range s.Contact {
		b.WriteString("Contact: " + c + "\n")
	}
	b.WriteString("Expires: " + at.UTC().Format(time.RFC3339) + "\n")
	if s.Policy != "" {
		b.WriteString("Policy: " + s.Policy + "\n")
	}
	if len(s.PreferredLanguages) > 0 {
		b.WriteString("Preferred-Languages: " + strings.Join(s.PreferredLanguages, ", ") + "\n")
	}
	return b.String()
}

// Warnings names what a role should log at boot about the published file, one
// sentence each. An expired file is still served — it is the operator's
// statement, and RFC 9116 leaves the staleness judgement to the reader — but a
// boot that says nothing would let it lapse unnoticed.
func (w Web) Warnings(now time.Time) []string {
	if w.SecurityTxt == nil {
		return nil
	}
	at, err := w.SecurityTxt.expiry()
	if err != nil {
		return nil
	}
	switch {
	case !at.After(now):
		return []string{fmt.Sprintf("web.security_txt.expires (%s) is not in the future, so readers treat /.well-known/security.txt as stale. Move it forward in margince.yaml.", w.SecurityTxt.Expires)}
	case at.Sub(now) > securityTxtHorizon:
		return []string{fmt.Sprintf("web.security_txt.expires (%s) is more than a year away; RFC 9116 recommends less, so the contact is reviewed.", w.SecurityTxt.Expires)}
	}
	return nil
}
