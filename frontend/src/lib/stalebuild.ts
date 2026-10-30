// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readStored, STORAGE_KEYS, writeStored } from "./storage";

// A tab opened before a deploy runs the previous build. Its screens load their
// code on first visit, by file names that build hashed. The deploy replaced
// those files, so the first screen the tab had not opened answers 404. Only a
// reload fixes it: React's `lazy` keeps the failed import, so a retry fails too.

// A reload onto a build that still cannot load the screen must not reload
// again. The next deploy later in the day still gets its own reload.
const RELOAD_GUARD_MS = 60_000;

// What Chromium, Firefox and Safari say when a module script is missing, and
// what Vite says when the stylesheet it preloads beside a chunk is.
const STALE_CHUNK =
  /Failed to fetch dynamically imported module|error loading dynamically imported module|Importing a module script failed|Unable to preload CSS/;

export function isStaleChunkError(error: unknown): boolean {
  return error instanceof Error && STALE_CHUNK.test(error.message);
}

/** Reloads the page onto the build now being served, unless this tab already
 *  did so in the last minute. Reports whether it reloaded. */
export function reloadForNewBuild(now: number = Date.now()): boolean {
  const key = STORAGE_KEYS.staleBuildReloadedAt;
  const last = Number(readStored(key));
  if (last > 0 && now - last < RELOAD_GUARD_MS) {
    return false;
  }
  writeStored(key, String(now));
  // Storage that refused the write cannot guard a loop, so the page does not
  // reload. The view's own fallback still tells the reader to.
  if (readStored(key) !== String(now)) {
    return false;
  }
  window.location.reload();
  return true;
}

/** Vite announces a chunk or its stylesheet failing to load before the import
 *  rejects. Reloading there replaces the page before the view ever fails. */
export function reloadOnStaleBuild(): void {
  window.addEventListener("vite:preloadError", (event) => {
    if (reloadForNewBuild()) {
      event.preventDefault();
    }
  });
}
