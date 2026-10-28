// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Milestones of loading the app, as browser performance marks that the Home
// benchmark (`make bench-home`) reads. Each is set once per page load, so a
// remount or a refetch cannot move it. Nothing is sent anywhere.

export type JourneyMark =
  | "session"
  | "gates"
  | "home:useful"
  | "home:failed"
  | "home:complete";

export function journeyMarkName(mark: JourneyMark): string {
  return `margince:${mark}`;
}

export function markJourney(mark: JourneyMark): void {
  const name = journeyMarkName(mark);
  if (performance.getEntriesByName(name, "mark").length > 0) {
    return;
  }
  performance.mark(name);
}
