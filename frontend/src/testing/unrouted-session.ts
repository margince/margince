// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The signal that something mounted a capability-aware surface and left
// `GET /me` unrouted, recorded where the test runner can act on it.
//
// story-utils' fetch stub already refuses to guess a session and says so on
// console.error, which the render gate treats as fatal — so no STORY reaches
// review with an unrouted session. A vitest case could: the same warning
// printed into a wall of output, the same denied branch drawn, and the only
// thing deciding which branch a case asserted against was whether the failed
// session query settled before the assertion ran. Under load it did not, and
// three onboarding cases failed on three different names across four full runs
// and passed on every re-run — the shape that teaches a reader to re-run
// rather than read.
//
// A counter rather than a thrown error, because the stub answers from inside a
// queryFn: a throw there becomes a query error, which is the same silent denied
// branch it was trying to report. Only the runner, after the case, can turn it
// into a failure — vitest.setup.ts does, and scripts/unroutedsession.test.tsx
// is what fails if that stops.
//
// A case that MEANS to exercise the unrouted branch calls `takeUnroutedSessionProbes`
// itself; taking the count is what says so.

let probes = 0;

/** Called by the fetch stub when it had to refuse a session it cannot guess. */
export function recordUnroutedSessionProbe(): void {
  probes += 1;
}

/** The count since it was last taken, and resets it. */
export function takeUnroutedSessionProbes(): number {
  const taken = probes;
  probes = 0;
  return taken;
}
