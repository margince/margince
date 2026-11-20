// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"net/http"
	"strconv"
	"strings"
)

// IfMatchVersion reads the optional If-Match row version (a bare
// integer, not a quoted ETag). Malformed input, an empty value
// included, is a client error, not last-write-wins.
func IfMatchVersion(w http.ResponseWriter, r *http.Request) (*int64, bool) {
	values := r.Header.Values("If-Match")
	if len(values) == 0 {
		return nil, true
	}
	// A present but empty header is a malformed version, never "no check", and
	// a second header line would otherwise hide behind the first.
	var v int64
	var err error
	if len(values) == 1 {
		v, err = strconv.ParseInt(values[0], 10, 64)
	}
	if len(values) != 1 || err != nil || v < 1 {
		Write(w, r, Validation("If-Match", "malformed_if_match", "If-Match carries the last-seen integer version"))
		return nil, false
	}
	return &v, true
}

// IfNoneMatchHit reports whether r's If-None-Match already names etag (a
// quoted strong validator, e.g. `"abc123"`), so a caller can answer 304
// before doing the work a fresh response would need. The header is a
// comma-separated list and "*" matches anything. Comparison is weak, so a
// proxy's `W/` prefix on this server's strong tag still matches. The list is
// split outside quotes, because a comma is a legal byte inside an etag-value.
// Several header lines are read through Header.Values.
func IfNoneMatchHit(r *http.Request, etag string) bool {
	for _, line := range r.Header.Values("If-None-Match") {
		for _, candidate := range splitUnquoted(line, ',') {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" {
				return true
			}
			if strings.TrimPrefix(candidate, "W/") == strings.TrimPrefix(etag, "W/") {
				return true
			}
		}
	}
	return false
}

// splitUnquoted splits s on sep, except inside a double-quoted span, which an
// ETag list needs because an etag-value may hold a comma.
func splitUnquoted(s string, sep byte) []string {
	var parts []string
	inQuotes := false
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case sep:
			if !inQuotes {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}
