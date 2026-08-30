// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one place that decides whether a string may become a link.
//
// Every href a record carries is untrusted input: a crawler wrote it, a
// connector wrote it, or a person pasted it into a field whose type set has no
// `url` member. `javascript:` and `data:` in an href are script execution on
// click, and a value that is not an absolute URL at all — a bare
// `example.com`, a relative path — resolves against OUR origin, which is never
// where the record points. So only the two schemes a web address can honestly
// be are allowed through, and everything else is text.
//
// It answers with the parsed URL rather than a boolean so a caller that needs
// the normalized destination has it, and one that only needs the verdict can
// ask for a null check. What each surface DRAWS for a refused value is the
// caller's own decision — the chip keeps the fact and drops the link, the
// custom-field cell stays plain text — and the schemes are not.
export function webUrl(value: string): URL | null {
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    // Unparseable as an absolute URL, which is what most values are. That is
    // the answer "this is text", not a failure to report.
    return null;
  }
  return parsed.protocol === "https:" || parsed.protocol === "http:"
    ? parsed
    : null;
}

// The hosts a LinkedIn profile can honestly live on.
const LINKEDIN_HOSTS = ["linkedin.com", "lnkd.in"];

// Whether a stored profile address may be shown under the word "LinkedIn".
//
// A link's LABEL is a claim about where it goes, and these labels are fixed
// words rather than the address itself. `social` is an open map on the wire and
// a profile URL can be written by a crawl, a connector or a paste, so without a
// host check a value of `https://attacker.example/login` renders as a link
// reading "LinkedIn" inside the product's own chrome — which is a better
// phishing surface than an email, because the reader already trusts the frame.
//
// A refused value is not hidden: the caller keeps the fact and drops the link,
// the same way `webUrl` leaves an unparseable address as text. The fact that a
// contact has SOMETHING recorded is still worth showing; the claim that it is
// LinkedIn is what this withholds.
export function linkedinUrl(value: string): URL | null {
  const parsed = webUrl(value);
  if (!parsed) {
    return null;
  }
  const host = parsed.hostname.toLowerCase();
  const onHost = LINKEDIN_HOSTS.some(
    (allowed) => host === allowed || host.endsWith(`.${allowed}`),
  );
  return onHost ? parsed : null;
}
