// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

type Address = Readonly<{ pathname: string; search: string; hash: string }>;

// A shared /book/<slug>?l=N link carries the length on the path's query; the
// app's own #/book/<slug>?l=N carries it on the fragment's.
function addressLength(address: Address): number | undefined {
  const raw =
    new URLSearchParams(address.search).get("l") ??
    new URLSearchParams(address.hash.split("?")[1]).get("l");
  const length = Number(raw);
  return Number.isInteger(length) && length > 0 ? length : undefined;
}

/** The address with its meeting length set to length, in both queries it may sit in. */
export function withLength(address: Address, length: number): string {
  const search = new URLSearchParams(address.search);
  search.set("l", String(length));
  const [route, query] = address.hash.split("?");
  const fragment = new URLSearchParams(query);
  if (fragment.has("l")) fragment.set("l", String(length));
  const hash = fragment.size > 0 ? `${route}?${fragment}` : route;
  return `${address.pathname}?${search}${hash}`;
}

/**
 * The meeting length a guest asked for: the address's `l` at first, then the
 * one they pick, which the address keeps so a reload or a shared copy opens
 * at it. A personal page (a proposal, the host's preview) has no length to pick.
 */
export function useGuestLength(personal: boolean) {
  const [length, setLength] = useState(() =>
    personal ? undefined : addressLength(globalThis.location),
  );
  const pick = (next: number) => {
    setLength(next);
    globalThis.history.replaceState(
      globalThis.history.state,
      "",
      withLength(globalThis.location, next),
    );
  };
  return [length, pick] as const;
}
