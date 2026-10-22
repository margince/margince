// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { parseHash } from "../../lib/router";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";

// A provider sign-in is a full-page trip, and the callback lands on the bare
// post-login URL, so the route a reader followed survives only in this tab.

/** True when the address names no route, so sign-in may choose one. */
export function isBareEntry(hash: string): boolean {
  return hash === "" || hash === "#" || hash === "#/";
}

// Only this app's own hash route is kept; it is written back as a fragment of
// the current page, so no stored value can move the tab to another origin.
function isOwnRoute(hash: string): boolean {
  return (
    hash.startsWith("#/") &&
    !isBareEntry(hash) &&
    parseHash(hash).screen !== "not-found"
  );
}

/** Remember the route the reader is on, before the page leaves for a provider. */
export function keepReturnRoute(): void {
  const hash = globalThis.location?.hash ?? "";
  if (isOwnRoute(hash)) {
    writeStored(STORAGE_KEYS.signInReturnRoute, hash);
  } else {
    removeStored(STORAGE_KEYS.signInReturnRoute);
  }
}

/**
 * Put the remembered route back on the bare landing, once. Any load spends it,
 * so a later visit to the bare address in this tab opens Home as usual.
 */
export function restoreReturnRoute(): void {
  const route = readStored(STORAGE_KEYS.signInReturnRoute);
  if (route === null) {
    return;
  }
  removeStored(STORAGE_KEYS.signInReturnRoute);
  const { hash, pathname, search } = globalThis.location;
  if (isBareEntry(hash) && isOwnRoute(route)) {
    globalThis.history.replaceState(null, "", `${pathname}${search}${route}`);
  }
}
