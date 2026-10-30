// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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

function recordPath(hash: string): string {
  const record = RECORD.exec(hash);
  return record ? `/r/${record[1]}/${record[2]}` : "/";
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
    (link.pathname !== "/" && !RECORD_PATH.test(link.pathname))
  )
    return;
  event.preventDefault();
  location.hash = link.hash;
}

// A browser can enter the app directly at the same address a preview fetcher reads.
export function restoreRecordAddress(): void {
  if (location.hash) return;
  const record = RECORD_PATH.exec(location.pathname);
  if (record)
    history.replaceState(
      history.state,
      "",
      `${location.pathname}${location.search}#/${record[1]}/${record[2]}`,
    );
}

// Keep the hash router while making the record identity visible to preview fetchers.
export function publishRecordAddress(): void {
  const { pathname, search, hash } = globalThis.location;
  if (pathname !== "/" && !pathname.startsWith("/r/")) return;
  const next = recordPath(hash);
  if (pathname !== next)
    history.replaceState(history.state, "", `${next}${search}${hash}`);
}
