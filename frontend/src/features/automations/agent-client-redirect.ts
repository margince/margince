// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The server's rule for a redirect URI a client may register, as
// `validRedirectURI` in backend/internal/modules/identity/oauth_redirect.go.
// The form refuses a URI as it is added, not after a round trip.
// testdata/redirecturi.json beside the Go rule holds both to one corpus.

// Go's Hostname() drops the brackets of an IPv6 literal; this reads them raw.
const LOOPBACK_HOSTS = new Set(["localhost", "127.0.0.1", "[::1]"]);

// Scheme, then the host of an authority, read from the text as typed. `URL`
// lower-cases a host and adds the `//` an opaque `https:x` lacks; Go does
// neither.
const AUTHORITY =
  /^([a-z][a-z0-9+.-]*):\/\/(?:[^@/?#]*@)?(\[[^\]/?#]*\]|[^:/?#]*)/i;

export function acceptsRedirectURI(raw: string): boolean {
  const authority = AUTHORITY.exec(raw);
  if (authority === null || !URL.canParse(raw)) return false;
  const [, scheme = "", host = ""] = authority;
  const hash = raw.indexOf("#");
  if (hash !== -1 && hash < raw.length - 1) return false;
  if (!decodableQuery(raw.slice(0, hash === -1 ? undefined : hash))) {
    return false;
  }
  switch (scheme.toLowerCase()) {
    case "https":
      return host !== "";
    case "http":
      return LOOPBACK_HOSTS.has(host);
    default:
      return false;
  }
}

// Go's url.ParseQuery refuses a semicolon and a broken percent escape.
function decodableQuery(beforeFragment: string): boolean {
  const mark = beforeFragment.indexOf("?");
  if (mark === -1) return true;
  const query = beforeFragment.slice(mark + 1);
  if (query.includes(";")) return false;
  try {
    for (const part of query.split(/[&=]/)) {
      decodeURIComponent(part.replaceAll("+", " "));
    }
  } catch {
    return false;
  }
  return true;
}
