// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The RFC 9116 contact file this installation publishes. The contact belongs
// to whoever operates the installation, so nothing is compiled in: unset, the
// route is not mounted and answers 404.

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/language"

	"github.com/margince/margince/backend/internal/platform/config"
)

// SecurityTxt is the file, field by field.
type SecurityTxt struct {
	// Contact is one or more URIs a finder reports to: mailto:, https: or tel:.
	Contact []string `yaml:"contact" json:"contact"`
	// Expires is the RFC 3339 instant after which a reader treats the file as stale.
	Expires string `yaml:"expires" json:"expires"`
	// Policy is an optional https: link to the disclosure policy.
	Policy string `yaml:"policy" json:"policy"`
	// PreferredLanguages is an optional list of language tags a report may be written in.
	PreferredLanguages []string `yaml:"preferred_languages" json:"preferred_languages"`
}

// WebSecurityTxt is served at /.well-known/security.txt when set.
var WebSecurityTxt = config.Define(config.Spec[*SecurityTxt]{
	Key: "web.security_txt", Env: "MARGINCE_WEB_SECURITY_TXT", Type: config.Struct[*SecurityTxt](),
	Default: config.Some[*SecurityTxt](nil), Validate: validSecurityTxt, Roles: []config.Role{config.RoleAPI},
	Doc: "RFC 9116 security.txt: one or more contact URIs and an RFC 3339 end time, with an optional policy link and preferred_languages.",
})

// languageTag is the character set of a BCP 47 tag. It refuses the comma and
// the line break a value would otherwise smuggle into the rendered file, and
// the underscore language.Parse forgives.
var languageTag = regexp.MustCompile(`^[A-Za-z]{1,8}(-[A-Za-z0-9]{1,8})*$`)

// rfc3339 is RFC 3339 §5.6. time.Parse alone takes a comma before the
// fraction, which the served file would restate as something never written.
var rfc3339 = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// securityTxtHorizon is RFC 9116's advice: Expires less than a year ahead, so
// the file is looked at again.
const securityTxtHorizon = 365 * 24 * time.Hour

func validSecurityTxt(s *SecurityTxt) error {
	if s == nil {
		return nil
	}
	if len(s.Contact) == 0 {
		return errors.New("web.security_txt.contact needs at least one mailto:, https: or tel: URI")
	}
	for _, c := range s.Contact {
		if err := checkURI(c, "mailto", schemeHTTPS, "tel"); err != nil {
			return fmt.Errorf("web.security_txt.contact %q: %w", c, err)
		}
	}
	if _, err := s.expiry(); err != nil {
		return err
	}
	if s.Policy != "" {
		if err := checkURI(s.Policy, schemeHTTPS); err != nil {
			return fmt.Errorf("web.security_txt.policy %q: %w", s.Policy, err)
		}
	}
	for _, lang := range s.PreferredLanguages {
		if _, err := language.Parse(lang); err != nil || !languageTag.MatchString(lang) {
			return fmt.Errorf("web.security_txt.preferred_languages %q is not a language tag such as en or de-CH", lang)
		}
	}
	return nil
}

// expiry parses Expires; RFC 9116 makes the field mandatory.
func (s SecurityTxt) expiry() (time.Time, error) {
	if s.Expires == "" {
		return time.Time{}, errors.New("web.security_txt.expires is required (RFC 3339, e.g. 2030-01-01T00:00:00Z)")
	}
	at, err := time.Parse(time.RFC3339, s.Expires)
	if err != nil || !rfc3339.MatchString(s.Expires) {
		return time.Time{}, fmt.Errorf("web.security_txt.expires %q is not an RFC 3339 date-time such as 2030-01-01T00:00:00Z", s.Expires)
	}
	return at, nil
}

// checkURI holds a value to an absolute URI in one of schemes, with a host for
// https. url.Parse refuses control characters and this refuses whitespace, so
// a value cannot add a line of its own to the rendered file.
func checkURI(raw string, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || strings.ContainsAny(raw, " \t") {
		return errors.New("is not a URI")
	}
	for _, scheme := range schemes {
		if u.Scheme != scheme {
			continue
		}
		if scheme == schemeHTTPS && u.Host == "" {
			return errors.New("names no host")
		}
		if scheme != schemeHTTPS && u.Opaque == "" {
			return errors.New("names no address")
		}
		if scheme == "mailto" && !strings.Contains(u.Opaque, "@") {
			return errors.New("names no mailbox")
		}
		return nil
	}
	return fmt.Errorf("is not a %s: URI", strings.Join(schemes, ":, "))
}

// securityTxtBody renders the served file, or "" when none is configured,
// which leaves the route unmounted. It assumes a value validSecurityTxt took.
func securityTxtBody(s *SecurityTxt) string {
	if s == nil {
		return ""
	}
	at, err := s.expiry()
	if err != nil {
		return ""
	}
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

// securityTxtWarnings names what a boot should log about the file. An expired
// file is still served, since RFC 9116 leaves staleness to the reader, but a
// silent boot would let it lapse unnoticed.
func securityTxtWarnings(s *SecurityTxt, now time.Time) []string {
	if s == nil {
		return nil
	}
	at, err := s.expiry()
	if err != nil {
		return nil
	}
	switch {
	case !at.After(now):
		return []string{fmt.Sprintf("web.security_txt.expires (%s) is not in the future, so readers treat /.well-known/security.txt as stale. Move it forward in margince.yaml.", s.Expires)}
	case at.Sub(now) > securityTxtHorizon:
		return []string{fmt.Sprintf("web.security_txt.expires (%s) is more than a year away; RFC 9116 recommends less, so the contact is reviewed.", s.Expires)}
	}
	return nil
}
