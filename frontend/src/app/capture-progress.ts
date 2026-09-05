// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

type CaptureConnection = components["schemas"]["CaptureConnection"];
type BackfillStatus = components["schemas"]["BackfillStatus"];

/**
 * Mail being taken in RIGHT NOW, across every mailbox this person connected.
 *
 * A connect-time import is the one long run the capture pipeline makes on a
 * person's behalf, and it is the run the AI-activity feed cannot carry: the
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
   * The count the person consented to, summed over the runs that previewed one;
   * null when no live run carries an estimate, and then there is no fraction
   * to draw.
   */
  estimated: number | null;
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
  const estimates = live.flatMap(({ run }) =>
    run.estimated_messages !== null &&
    run.estimated_messages !== undefined &&
    run.estimated_messages > 0
      ? [run.estimated_messages]
      : [],
  );
  const estimated =
    estimates.length === 0
      ? null
      : estimates.reduce((sum, count) => sum + count, 0);
  return {
    scanned,
    estimated,
    fraction:
      estimated === null ? null : Math.max(0, Math.min(1, scanned / estimated)),
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
