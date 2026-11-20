// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The server's rule for a redirect URI a client may register, as
// `validRedirectURI` in backend/internal/modules/identity/oauth_redirect.go.
// The form refuses a URI as it is added, not after a round trip.
// testdata/redirecturi.json beside the Go rule holds both to one corpus.

// The browser's `URL` repairs a tab Go refuses and refuses a port Go takes, so
// the parts of Go's `url.Parse` the rule reads are spelled out here.

const LOOPBACK_HOSTNAMES = new Set(["localhost", "127.0.0.1", "::1"]);

// A letter, then letters, digits, `+`, `-` or `.`, then the colon.
const SCHEME = /^([a-zA-Z][a-zA-Z0-9+.-]*):/;

const ALNUM = /^[a-zA-Z0-9]$/;

// The characters Go leaves unescaped in a host, beside letters and digits.
const HOST_MARKS = new Set("!$&'()*+,;=:[]<>\"-_.~");

// The characters Go accepts in userinfo, beside letters and digits.
const USERINFO_MARKS = new Set("-._:~!$&'()*+,;=%@");

type Mode = "host" | "path" | "query" | "fragment" | "userinfo";

type Parsed = Readonly<{
  scheme: string;
  host: string;
  rawQuery: string;
  fragment: string;
}>;

export function acceptsRedirectURI(raw: string): boolean {
  const parsed = parse(raw);
  if (parsed === null || parsed.fragment !== "") return false;
  if (!queryParses(parsed.rawQuery)) return false;
  switch (parsed.scheme) {
    case "https":
      return parsed.host !== "";
    case "http":
      return LOOPBACK_HOSTNAMES.has(hostname(parsed.host));
    default:
      return false;
  }
}

function parse(raw: string): Parsed | null {
  if (holdsControlByte(raw)) return null;
  const [beforeFragment, rawFragment] = cut(raw, "#");
  const fragment =
    rawFragment === undefined ? "" : decode(rawFragment, "fragment");
  const scheme = SCHEME.exec(beforeFragment);
  if (fragment === null || scheme === null) return null;
  const [rest, rawQuery = ""] = cut(
    beforeFragment.slice(scheme[0].length),
    "?",
  );
  const lowered = (scheme[1] ?? "").toLowerCase();
  // With no `//` there is no authority: an opaque URI has no host.
  if (!rest.startsWith("//")) {
    return { scheme: lowered, host: "", rawQuery, fragment };
  }
  const slash = rest.indexOf("/", 2);
  const authority = slash === -1 ? rest.slice(2) : rest.slice(2, slash);
  const path = slash === -1 ? "" : rest.slice(slash);
  const host = parseAuthority(authority);
  if (host === null || decode(path, "path") === null) return null;
  return { scheme: lowered, host, rawQuery, fragment };
}

function parseAuthority(authority: string): string | null {
  const at = authority.lastIndexOf("@");
  if (at !== -1) {
    const userinfo = authority.slice(0, at);
    if (!validUserinfo(userinfo) || decode(userinfo, "userinfo") === null) {
      return null;
    }
  }
  return parseHost(authority.slice(at + 1));
}

function parseHost(host: string): string | null {
  if (host.startsWith("[")) {
    const close = host.lastIndexOf("]");
    if (close === -1 || !validOptionalPort(host.slice(close + 1))) return null;
    const [address, zone] = cut(host.slice(1, close), "%25");
    if (!isIPv6(address)) return null;
    if (zone !== undefined && decode(zone, "host") === null) return null;
    return host;
  }
  const colon = host.lastIndexOf(":");
  if (colon !== -1) {
    if (!validOptionalPort(host.slice(colon))) return null;
    // Go refuses a second colon outside brackets.
    if (host.slice(0, colon).includes(":")) return null;
  }
  return decode(host, "host");
}

// Go's `url.URL.Hostname`: the host without its port or brackets.
function hostname(host: string): string {
  const colon = host.lastIndexOf(":");
  const bare =
    colon !== -1 && validOptionalPort(host.slice(colon))
      ? host.slice(0, colon)
      : host;
  return bare.startsWith("[") && bare.endsWith("]") ? bare.slice(1, -1) : bare;
}

function validOptionalPort(port: string): boolean {
  return port === "" || /^:\d*$/.test(port);
}

function validUserinfo(userinfo: string): boolean {
  return [...userinfo].every(
    (char) => ALNUM.test(char) || USERINFO_MARKS.has(char),
  );
}

// Go's `url.ParseQuery`: a semicolon or a broken escape refuses the query.
function queryParses(rawQuery: string): boolean {
  return rawQuery
    .split("&")
    .every(
      (part) =>
        !part.includes(";") &&
        part.split("=").every((side) => decode(side, "query") !== null),
    );
}

// Go's `stringContainsCTLByte`: a URI holding one does not parse.
function holdsControlByte(text: string): boolean {
  return [...text].some((char) => {
    const code = char.charCodeAt(0);
    return code < 0x20 || code === 0x7f;
  });
}

// Go's `unescape`, which decodes bytes and never asks for valid UTF-8. Null
// where Go answers an error.
function decode(text: string, mode: Mode): string | null {
  let out = "";
  for (let at = 0; at < text.length; at += 1) {
    const char = text.charAt(at);
    if (char === "%") {
      const byte = escapedByte(text.slice(at + 1, at + 3), mode);
      if (byte === null) return null;
      out += byte;
      at += 2;
    } else if (mode === "host" && refusedInHost(char)) {
      return null;
    } else {
      out += mode === "query" && char === "+" ? " " : char;
    }
  }
  return out;
}

// A host takes an escape only for a non-ASCII byte, or for `%25` itself.
function escapedByte(pair: string, mode: Mode): string | null {
  if (!/^[0-9a-fA-F]{2}$/.test(pair)) return null;
  const byte = Number.parseInt(pair, 16);
  if (mode === "host" && byte < 0x80 && pair !== "25") return null;
  return String.fromCharCode(byte);
}

function refusedInHost(char: string): boolean {
  return (
    char.charCodeAt(0) < 0x80 && !ALNUM.test(char) && !HOST_MARKS.has(char)
  );
}

// An IPv6 literal as Go reads one: eight groups of up to four hex digits. One
// `::` stands for the groups left out, and an IPv4 tail counts as two.
function isIPv6(address: string): boolean {
  const halves = address.split("::");
  if (halves.length > 2) return false;
  const groups = halves.flatMap((half) => (half === "" ? [] : half.split(":")));
  const tail = groups.at(-1) ?? "";
  const withV4 = tail.includes(".");
  if (withV4 && !isIPv4(tail)) return false;
  const hex = withV4 ? groups.slice(0, -1) : groups;
  if (!hex.every((group) => /^[0-9a-fA-F]{1,4}$/.test(group))) return false;
  const count = hex.length + (withV4 ? 2 : 0);
  return halves.length === 2 ? count < 8 : count === 8;
}

function isIPv4(address: string): boolean {
  const parts = address.split(".");
  return (
    parts.length === 4 &&
    parts.every((part) => /^(0|[1-9]\d{0,2})$/.test(part) && Number(part) < 256)
  );
}

function cut(text: string, separator: string): [string, string | undefined] {
  const at = text.indexOf(separator);
  return at === -1
    ? [text, undefined]
    : [text.slice(0, at), text.slice(at + separator.length)];
}
