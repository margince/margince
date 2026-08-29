// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package capabilitypath answers one question for every log site in the
// tree: what may this request path say once it is written down?
//
// Some public routes carry a bearer credential in a path segment, because
// they are reached with no login at all — the recipient of an email follows
// a link and the token in it IS the authorization. Anyone who can read a log
// line holding one of those paths therefore holds a working credential: an
// ops dashboard, a shipped aggregator, a third-party log service, a support
// engineer reading a paste.
//
// This package sits in shared/ rather than beside the access log because
// BOTH sides of the request need it and they cannot import each other:
// platform/httpserver writes the access log, platform/httperr writes the
// error logs, and httpserver already imports httperr. Whoever logs a path
// reaches this package instead of reaching each other.
//
// The prefix list lives HERE, next to the redaction, rather than travelling
// as an argument from the composition layer. That is the whole point: a list
// a caller passes is a list a caller forgets, and the access log has six
// mount sites of which one remembered.
package capabilitypath

import "strings"

// redacted stands in for a capability the log must not carry.
const redacted = "[redacted]"

// credentialPrefixes name the route prefixes whose NEXT path segment is a
// bearer credential rather than an identifier.
//
// A route belongs here when possession of the segment grants access. It does
// NOT belong here merely for being unguessable: the public booking page
// (/v1/public/booking/) carries a slug the host hands out deliberately, which
// resolves to free/busy slots and nothing else, and redacting it would cost
// the access log the only thing naming which booking page was hit while
// buying nothing.
var credentialPrefixes = []string{
	// The preference centre: the token resolves straight to a
	// (workspace, person) consent state and can change it.
	"/v1/public/preferences/",
	// The confirm-details link: the token opens the subject's own record,
	// so a token in a log line is a readable copy of somebody's personal
	// data sitting in operations.
	"/v1/public/confirm/",
}

// Redact replaces the ONE path segment that follows a credential prefix and
// keeps everything around it, so the line still says which route was asked
// for and what trailed the credential ("…/unsubscribe") without saying which
// credential was presented. A path that reaches no prefix, or that stops at
// the prefix with no segment after it, is returned unchanged — there is
// nothing there to leak.
func Redact(path string) string {
	for _, prefix := range credentialPrefixes {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		rest := strings.TrimPrefix(path, prefix)
		if rest == "" {
			continue
		}
		if _, trailing, found := strings.Cut(rest, "/"); found {
			return prefix + redacted + "/" + trailing
		}
		return prefix + redacted
	}
	return path
}

// CredentialPrefixes returns the prefixes carrying a credential segment. It
// exists so a gate can assert that every route mounted under a
// credential-bearing prefix is one this package knows about, rather than the
// list being a copy somebody keeps in step by hand.
func CredentialPrefixes() []string {
	out := make([]string, len(credentialPrefixes))
	copy(out, credentialPrefixes)
	return out
}
