// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { parseHash, type Screen } from "./router";
import {
  readStoredJson,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "./storage";

// A provider sign-in is a full-page trip that lands on the bare post-login URL.
// The route a reader followed survives it only in this tab's storage.

/** Where the callback sends a sign-in that did not complete. */
export const OIDC_FAILURE_HASH = "#/login?oidc=failed";

// Long enough for a provider's consent and second factor. A route kept for an
// attempt the reader gave up on must not open on a later, unrelated visit.
const KEPT_FOR_MS = 15 * 60 * 1000;

// The reset screen renders ahead of the session gate, so restoring it after a
// sign-in would put a signed-in reader back on a sign-in form.
const RESET_SCREEN: Screen = "reset-password";

type Kept = Readonly<{ route: string; keptAt: number }>;

/** True when the address names no route, so sign-in may choose one. */
export function isBareEntry(hash: string): boolean {
  return hash === "" || hash === "#" || hash === "#/";
}

// Only this app's own hash route is kept. It is written back as a fragment of
// the current page, so no stored value can move the tab to another origin.
function isOwnRoute(hash: string): boolean {
  if (!hash.startsWith("#/") || isBareEntry(hash)) {
    return false;
  }
  const { screen } = parseHash(hash);
  return screen !== "not-found" && screen !== RESET_SCREEN;
}

function asKept(value: unknown): Kept | null {
  if (
    typeof value !== "object" ||
    value === null ||
    !("route" in value) ||
    !("keptAt" in value)
  ) {
    return null;
  }
  const { route, keptAt } = value;
  return typeof route === "string" && typeof keptAt === "number"
    ? { route, keptAt }
    : null;
}

/** Remember the route the reader is on, before the page leaves for a provider. */
export function keepReturnRoute(): void {
  const hash = globalThis.location?.hash ?? "";
  if (isOwnRoute(hash)) {
    const kept: Kept = { route: hash, keptAt: Date.now() };
    writeStored(STORAGE_KEYS.signInReturnRoute, JSON.stringify(kept));
  } else {
    removeStored(STORAGE_KEYS.signInReturnRoute);
  }
}

/** The kept route, if one is fresh and safe, forgotten as it is read. */
export function takeReturnRoute(): string | null {
  const kept = readStoredJson(STORAGE_KEYS.signInReturnRoute, asKept);
  removeStored(STORAGE_KEYS.signInReturnRoute);
  if (kept === null || Date.now() - kept.keptAt > KEPT_FOR_MS) {
    return null;
  }
  return isOwnRoute(kept.route) ? kept.route : null;
}

/**
 * Put the kept route back on the bare landing. A failed sign-in leaves it to
 * the sign-in screen, which puts it back after reading its notice. Any other
 * landing names its own route and spends it.
 */
export function restoreReturnRoute(): void {
  const { hash, pathname, search } = globalThis.location;
  if (hash === OIDC_FAILURE_HASH) {
    return;
  }
  const route = takeReturnRoute();
  if (route !== null && isBareEntry(hash)) {
    globalThis.history.replaceState(null, "", `${pathname}${search}${route}`);
  }
}
