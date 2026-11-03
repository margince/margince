// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Milestones of loading the app, as browser performance marks that the Home
// benchmark (`make bench-home`) reads. Each is set once per journey, so a
// remount or a refetch cannot move it. Nothing is sent anywhere.
//
// A journey is the document's own load unless the benchmark opens a later one
// in the running document with a `JOURNEY_START` mark. Nothing in the app sets
// that mark, so outside the benchmark each milestone is set once per document.

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
