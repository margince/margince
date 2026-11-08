// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The ids under #/book/ that are not a host's public page. features/booking/
// book.tsx dispatches on each of them. The first two open a signed-in host's
// own controls. The prefixed ones carry a contact or meeting id, or a guest's
// private proposal or management token. None of them may become a
// server-visible path: a token in a path reaches the web tier's logs, a
// preview fetch and a canonical URL.
export const BOOKING_HOST_IDS: readonly string[] = ["contact", "preview"];
export const BOOKING_ID_PREFIXES: readonly string[] = [
  "contact-",
  "meeting-",
  "proposal-",
  "manage-",
];

// The characters a public booking slug is minted from (base64url, see
// SeedBookingPageTx). nginx.conf's /book/ location admits the same set, so a
// path the web tier routes to the booking preview is one the app reads too.
const SLUG = /^[A-Za-z0-9_-]{1,128}$/;

/** True for a host's reusable public page, which may live at /book/<slug>. */
export function isShareableBookingSlug(id: string | undefined): boolean {
  return Boolean(
    id &&
      SLUG.test(id) &&
      !BOOKING_HOST_IDS.includes(id) &&
      !BOOKING_ID_PREFIXES.some((prefix) => id.startsWith(prefix)),
  );
}

const BOOKING_PATH = /^\/book\/([^/]+)\/?$/;

/** The slug a /book/<slug> path names, or null for any other path. */
export function bookingSlugFromPath(pathname: string): string | null {
  const encoded = BOOKING_PATH.exec(pathname)?.[1];
  if (encoded === undefined) return null;
  let slug: string;
  try {
    // nginx matches the decoded path, so /book/%41bc reaches the same page.
    slug = decodeURIComponent(encoded);
  } catch {
    return null;
  }
  return isShareableBookingSlug(slug) ? slug : null;
}
