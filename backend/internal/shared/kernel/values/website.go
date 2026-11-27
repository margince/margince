// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"net/url"
	"strings"
)

// PublicWebsite reports whether typed text names a company's public site: a
// bare domain or an http(s) URL whose host is a dotted name ParseDomain takes.
// A bare word, an IP address or a localhost name is no site anyone can open.
func PublicWebsite(raw string) bool {
	candidate := strings.TrimSpace(raw)
	if !strings.Contains(candidate, "://") {
		candidate = "https://" + candidate
	}
	parsed, err := url.Parse(candidate)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host, err := ParseDomain(parsed.Hostname())
	if err != nil {
		return false
	}
	return !strings.HasSuffix(host.String(), ".localhost")
}
