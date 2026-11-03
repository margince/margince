// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { bookingSlugFromPath, isShareableBookingSlug } from "./bookingaddress";
import { opensElsewhere } from "./linkclick";

const IDENTIFIER =
  "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
const RECORD = new RegExp(
  `^#/(deals|companies|contacts)/(${IDENTIFIER})(?:[/?]|$)`,
);
const RECORD_PATH = new RegExp(
  `^/r/(deals|companies|contacts)/(${IDENTIFIER})$`,
);

export function appHref(href: string): string;
export function appHref(href: string | undefined): string | undefined;
export function appHref(href: string | undefined): string | undefined {
  if (!href) return href;
  const hash = href.startsWith("/#/") ? href.slice(1) : href;
  if (!hash.startsWith("#/")) return href;
  return `${recordPath(hash)}${hash}`;
}

const BOOKING = /^#\/book\/([^/?]+)(?:[/?]|$)/;

// The server-visible path an address mirrors: a record card, a host's public
// booking page, or the root. A booking id that is not a shareable slug (a
// guest's proposal or management token, a host control) always maps to the
// root, so it never leaves the fragment.
function recordPath(hash: string): string {
  const record = RECORD.exec(hash);
  if (record) return `/r/${record[1]}/${record[2]}`;
  const booking = BOOKING.exec(hash);
  if (booking && isShareableBookingSlug(booking[1]))
    return `/book/${booking[1]}`;
  return "/";
}

function isMirroredPath(pathname: string): boolean {
  return (
    pathname === "/" ||
    RECORD_PATH.test(pathname) ||
    bookingSlugFromPath(pathname) !== null
  );
}

type LinkPress = Readonly<{
  button: number;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  defaultPrevented: boolean;
  currentTarget: HTMLAnchorElement;
  preventDefault: () => void;
}>;

export function followAppLink(event: LinkPress): void {
  const link = event.currentTarget;
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    opensElsewhere(event) ||
    link.hasAttribute("download") ||
    (link.target && link.target !== "_self")
  )
    return;
  if (
    link.origin !== location.origin ||
    !link.hash.startsWith("#/") ||
    !isMirroredPath(link.pathname)
  )
    return;
  event.preventDefault();
  location.hash = link.hash;
}

// A browser can enter the app directly at the same address a preview fetcher reads.
export function restoreRecordAddress(): void {
  if (location.hash) return;
  const record = RECORD_PATH.exec(location.pathname);
  if (record) {
    history.replaceState(
      history.state,
      "",
      `${location.pathname}${location.search}#/${record[1]}/${record[2]}`,
    );
    return;
  }
  const slug = bookingSlugFromPath(location.pathname);
  if (slug)
    history.replaceState(
      history.state,
      "",
      `/book/${slug}${location.search}#/book/${slug}`,
    );
}

// Keep the hash router while making the record identity visible to preview fetchers.
export function publishRecordAddress(): void {
  const { pathname, search, hash } = globalThis.location;
  if (
    pathname !== "/" &&
    !pathname.startsWith("/r/") &&
    !pathname.startsWith("/book/")
  )
    return;
  const next = recordPath(hash);
  if (pathname !== next)
    history.replaceState(history.state, "", `${next}${search}${hash}`);
}
