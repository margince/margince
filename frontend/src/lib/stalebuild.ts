// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A tab opened before a deploy is running the previous build. Its screens load
// their code on first visit, by file names that build hashed — and the deploy
// replaced those files, so the first screen it has not opened yet answers 404
// and the view fails. Nothing is wrong with the screen or the server: the page
// is just older than what serves it, and only a reload fixes that. React's
// `lazy` keeps the failed import, so a retry inside the page fails the same way.

const RELOADED_AT_KEY = "margince.staleBuildReloadedAt";

// A reload that lands on a build that still cannot load the screen must not
// reload again. Long enough to cover one reload and its failure, short enough
// that the next deploy later in the day still gets its own reload.
const RELOAD_GUARD_MS = 60_000;

// What each engine says when a module script it was asked to import is not
// there: Chromium, Firefox and Safari in that order, and Vite's own wording for
// a stylesheet it preloads beside a chunk.
const STALE_CHUNK_MESSAGES = [
  "Failed to fetch dynamically imported module",
  "error loading dynamically imported module",
  "Importing a module script failed",
  "Unable to preload CSS",
];

export function isStaleChunkError(error: unknown): boolean {
  if (!(error instanceof Error)) {
    return false;
  }
  return STALE_CHUNK_MESSAGES.some((text) => error.message.includes(text));
}

/** Reloads the page onto the build now being served, unless this tab already
 *  did so in the last minute. Reports whether it reloaded. */
export function reloadForNewBuild(now: number = Date.now()): boolean {
  try {
    const last = Number(sessionStorage.getItem(RELOADED_AT_KEY));
    if (last > 0 && now - last < RELOAD_GUARD_MS) {
      return false;
    }
    sessionStorage.setItem(RELOADED_AT_KEY, String(now));
  } catch {
    // Without storage there is no guard, and an unguarded reload can loop. The
    // view's own fallback still tells the reader to reload.
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
