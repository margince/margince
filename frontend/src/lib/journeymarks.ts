// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Milestones of loading the app, as browser performance marks that the Home
// benchmark (`make bench-home`) reads. Each is set once per journey, so a
// remount or a refetch cannot move it. Nothing is sent anywhere.
//
// A journey is the document's own load unless the benchmark opens a later one
// in the running document with a `JOURNEY_START` mark. Nothing in the app sets
// that mark, so outside the benchmark each milestone is set once per document.

import { useSyncExternalStore } from "react";
import { parseHash } from "./router";

export type JourneyMark =
  | "session"
  | "gates"
  | "home:useful"
  | "home:failed"
  | "home:degraded"
  | "home:complete";

/** The mark that opens a journey; its `detail` is the journey's id. */
export const JOURNEY_START = "margince:journey";

export function journeyMarkName(mark: JourneyMark): string {
  return `margince:${mark}`;
}

/** The journey a mark belongs to: "" for the document's own load. */
export function journeyOf(entry: object): string {
  return "detail" in entry && typeof entry.detail === "string"
    ? entry.detail
    : "";
}

function currentJourney(): string {
  const start = performance.getEntriesByName(JOURNEY_START, "mark").at(-1);
  return start ? journeyOf(start) : "";
}

export function markJourney(mark: JourneyMark): void {
  if (mark === "home:useful" || mark === "home:failed") settleFirstScreen();
  const name = journeyMarkName(mark);
  const journey = currentJourney();
  const reached = performance
    .getEntriesByName(name, "mark")
    .some((entry) => journeyOf(entry) === journey);
  if (reached) {
    return;
  }
  performance.mark(name, { detail: journey === "" ? null : journey });
}

// The shell's side reads wait until Home's day is on screen. They then do not
// compete with the read and the paint Home is measured on. Another first
// screen, or a day that takes longer than the grace, lets them start anyway.
export const FIRST_SCREEN_GRACE_MS = 2_000;

let firstScreenSettled = false;
const settledListeners = new Set<() => void>();

/** Lets the shell's side reads start. Home calls it once its day is on screen. */
export function settleFirstScreen(): void {
  if (firstScreenSettled) return;
  firstScreenSettled = true;
  for (const listener of settledListeners) listener();
  settledListeners.clear();
}

if (
  typeof window === "undefined" ||
  parseHash(location.hash).screen !== "home"
) {
  settleFirstScreen();
} else {
  setTimeout(settleFirstScreen, FIRST_SCREEN_GRACE_MS);
}

function onFirstScreenSettled(listener: () => void): () => void {
  if (firstScreenSettled) return () => undefined;
  settledListeners.add(listener);
  return () => settledListeners.delete(listener);
}

/** Whether the shell's side reads may start (settleFirstScreen). */
export function useFirstScreenSettled(): boolean {
  return useSyncExternalStore(
    onFirstScreenSettled,
    () => firstScreenSettled,
    () => true,
  );
}
