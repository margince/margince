// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

/**
 * Mail being taken in RIGHT NOW, across every mailbox this contact connected.
 *
 * A connect-time import is the one long run the capture pipeline makes on a
 * contact's behalf, and it is the run the AI-activity feed cannot carry: the
 * feed's capture kinds are per-message classifications the router announces
 * once each call is over, so a two-hour import of a mailbox reaches the feed as
 * a trickle of settled lines and never as work in flight. What does say the
 * import is live is the run's own status row, embedded in every connection the
 * connectors read returns (`CaptureConnection.backfill`) — the same
 * persisted-row counts the settings card draws, read from the same body.
 *
 * ONE reading over every mailbox rather than one per mailbox, because the chrome
 * has one orb and one chip: two imports at once are one fact ("mail is being
 * captured") with one fraction, and the sources are named so the sentence can
 * still say whose.
 */
export type CaptureProgress = Readonly<{
  /** Messages the import has scanned, summed over every live run. */
  scanned: number;
  /**
   * The count the contact consented to, summed over the runs that previewed one;
   * null when no live run carries an estimate, and then there is no fraction
   * to draw.
   */
  estimated: number | null;
  /**
   * True when the denominator is a FLOOR: the provider stopped counting at its
   * cap, so the window holds at least `estimated` and how many more is
   * unknown. A surface printing the denominator says so.
   */
  estimatedIsFloor: boolean;
  /** `scanned / estimated` clamped to 0..1, or null without a denominator. */
  fraction: number | null;
  /** The mailboxes being imported, as the reader knows them. */
  sources: readonly string[];
}>;

/** How often the connectors read re-asks while an import is live. */
const POLL_LIVE_MS = 2_500;

function isLive(run: BackfillStatus | undefined): run is BackfillStatus {
  return (
    run !== undefined && (run.state === "running" || run.state === "queued")
  );
}

/**
 * The share of a window an import has read, or null when no honest share can be
 * drawn. ONE rule, because two surfaces draw this bar — the chrome's orb and
 * the settings card's — and a fraction that differed between them would be two
 * answers to one question on one screen.
 *
 * An EXACT denominator that was overrun clamps to 1: the mailbox grew between
 * the preview and the scan, and the run really is at its end. A FLOOR that has
 * been passed does not — the provider stopped counting at its cap, so nothing
 * knows how much is left, and a bar pinned full for the rest of a long import
 * says the import is complete when it is not. Past that point the surfaces fall
 * back to the absolute counts, which are true either way.
 */
export function progressFraction(
  reading: Readonly<{
    scanned: number;
    estimated: number | null;
    estimatedIsFloor: boolean;
  }>,
): number | null {
  const { scanned, estimated, estimatedIsFloor } = reading;
  if (estimated === null || estimated <= 0) {
    return null;
  }
  if (estimatedIsFloor && scanned >= estimated) {
    return null;
  }
  return Math.max(0, Math.min(1, scanned / estimated));
}

/**
 * The capture in flight, or null when no connection is importing.
 *
 * Null rather than a zero-progress reading, for the rail's standing rule: a
 * surface that reports the agent at work must reach that state because
 * something was read, and an all-zero reading would be this module inventing
 * an import nobody started.
 *
 * The fraction is clamped rather than trusted. `messages_scanned` is a
 * persisted-row count and `estimated_messages` was a preview: a mailbox that
 * grew between the preview and the scan reports more scanned than estimated,
 * and a ring past full is a ring drawn wrong.
 *
 * A FLOOR denominator is dropped rather than clamped once it is passed, and
 * that is the difference between the two. Clamping says the import is
 * complete; it is not, and nothing here knows how much is left — the provider
 * stopped counting at its cap. A ring pinned at full for the rest of a long
 * import is a worse statement than no ring at all, so past the floor the
 * surfaces fall back to absolute counts, decided by the fact the server sent
 * rather than by a client noticing an overrun.
 */
export function liveCapture(
  connections: readonly CaptureConnection[],
): CaptureProgress | null {
  const live = connections.flatMap((connection) =>
    isLive(connection.backfill)
      ? [{ connection, run: connection.backfill }]
      : [],
  );
  if (live.length === 0) {
    return null;
  }
  const scanned = live.reduce(
    (sum, { run }) => sum + (run.counts?.messages_scanned ?? 0),
    0,
  );
  const counted = live.flatMap(({ run }) =>
    run.estimated_messages !== null &&
    run.estimated_messages !== undefined &&
    run.estimated_messages > 0
      ? [run]
      : [],
  );
  const estimated =
    counted.length === 0
      ? null
      : counted.reduce((sum, run) => sum + (run.estimated_messages ?? 0), 0);
  // One floor in the sum makes the SUM a floor: the total is at least this,
  // and the mailbox that was capped could hold any number more.
  const estimatedIsFloor = counted.some(
    (run) => run.estimate_is_floor === true,
  );
  const passedTheFloor =
    estimatedIsFloor && estimated !== null && scanned >= estimated;
  return {
    scanned,
    estimated,
    estimatedIsFloor,
    fraction:
      estimated === null || passedTheFloor
        ? null
        : Math.max(0, Math.min(1, scanned / estimated)),
    sources: live.map(
      ({ connection }) => connection.account_label ?? connection.provider,
    ),
  };
}

/**
 * The cadence the connectors read should re-ask at, given its last answer.
 *
 * Live while any import runs, off otherwise: the read is what the orb, the
 * chip and the settings card draw the import from, and at the query client's
 * thirty-second staleness an import that lasted a minute would be reported at
 * two resolutions. `false` rather than the idle cadence, because at rest the
 * connections are a standing fact and every other reader of them already
 * refetches on its own terms.
 */
export function connectorsPollInterval(
  answer: Readonly<{ data: readonly CaptureConnection[] }> | undefined,
): number | false {
  return answer !== undefined && liveCapture(answer.data) !== null
    ? POLL_LIVE_MS
    : false;
}
