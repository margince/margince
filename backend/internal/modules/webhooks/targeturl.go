// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"net/url"
	"strings"
	"unicode"
)

// checkTargetURL refuses an address nothing can deliver to: not https, no host,
// or text a URL cannot hold. Go's parser takes a space in a path, so the text
// is judged before it is parsed.
func checkTargetURL(raw string) error {
	bad := &BadInputError{Field: "target_url", Reason: "must be an https:// URL with a host and no spaces"}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return bad
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return bad
	}
	return nil
}
