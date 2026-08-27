// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { webUrl } from "./weburl";

// What a person types when they mean a profile address, turned into one — and
// nothing more than that.
//
// The product never fetches a profile page: the user reads it in their own
// browser and stores what they learned. So this normalizes SHAPE only. It makes
// no request, resolves no redirect, and asks no third party whether the address
// exists. A pasted address that is merely wrong stays wrong until a person
// fixes it, which is the honest outcome when the only reader is the person who
// pasted it.
//
// `webUrl` still owns the verdict on whether the result may become an href.
// This runs BEFORE it, turning the two things people actually paste into
// something `webUrl` can judge: a bare host ("linkedin.com/in/jdoe", copied
// from a browser that hides the scheme) and a stray wrapper of whitespace.

// Prepending https to a bare host is a guess, and it is only ever made for a
// value that has no scheme at all. A value carrying `http://` keeps it: the
// caller typed a scheme, and quietly upgrading it would claim a certificate we
// have not seen.
const BARE_HOST = /^[a-z0-9-]+(\.[a-z0-9-]+)+(\/|$)/i;

export function normalizeProfileUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }
  const withScheme = BARE_HOST.test(trimmed) ? `https://${trimmed}` : trimmed;
  const parsed = webUrl(withScheme);
  if (!parsed) {
    // Not an address this product would ever link. Hand back what the person
    // typed rather than a mangled version of it — the field then shows their
    // own text, which is what they need in order to correct it.
    return trimmed;
  }
  // A profile address copied from a browser carries the session's own query
  // junk (tracking parameters, an originating-page marker). Two people copying
  // the same profile from different pages would otherwise store two values
  // that no comparison could match, and the parameters say nothing about the
  // person. The fragment goes for the same reason.
  parsed.search = "";
  parsed.hash = "";
  // A trailing slash is the one difference the browser itself introduces
  // between a typed address and a copied one.
  if (parsed.pathname.length > 1 && parsed.pathname.endsWith("/")) {
    parsed.pathname = parsed.pathname.slice(0, -1);
  }
  return parsed.toString();
}

// The address as a person reads it: no scheme, no `www.`, no trailing slash.
//
// A field's value column is not a place for "https://" — it is chrome the
// reader already knows, and at a field's width it costs the part that
// identifies the profile. The full address stays in the href.
export function profileUrlLabel(raw: string): string {
  const parsed = webUrl(normalizeProfileUrl(raw));
  if (!parsed) {
    return raw.trim();
  }
  const host = parsed.host.replace(/^www\./, "");
  const path = parsed.pathname === "/" ? "" : parsed.pathname;
  return `${host}${path}`;
}
