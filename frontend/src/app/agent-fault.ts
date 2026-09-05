// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useMemo, useState } from "react";
import type { components } from "../api/schema";

type AiActivityItem = components["schemas"]["AiActivityItem"];

/**
 * The run that broke, held on the orb until somebody has actually looked at it.
 *
 * A scheduled run fails at four in the morning and the person it ran for is
 * asleep. Whatever the orb does at 04:12 is seen by nobody, so a fault that
 * decays on a timer, or on the next run settling, is a fault the reader can miss
 * entirely: the product knew its overnight work failed and never said so.
 *
 * So the rule is acknowledgement, not decay. The newest unacknowledged failure
 * holds the orb, and opening the panel is what acknowledges it: the panel lists
 * the run, in the reader's own words, with whatever the source said about why it
 * stopped. Once seen it drops out of the orb and stays in the panel's settled
 * list for the rest of the day, which is where a fault belongs after it has been
 * delivered once.
 *
 * The seen marks are per browser and nothing else. There is no server-side read
 * or write for "this person has been told", and inventing one here would be a
 * durable claim about a person built out of one tab's local storage. What the
 * mark actually buys is the thing it can honestly buy: a reload, or a second
 * screen in the same browser, does not raise a fault the reader already dealt
 * with.
 */
const SEEN_KEY = "margince.agent.faults-seen";

/**
 * How many acknowledgements are kept. Faults are rare and the arm behind them is
 * bounded to one local day, so a short list outlives every id that could still be
 * raised, and the oldest falling off can only ever re-raise a run from a previous
 * day that the feed no longer carries.
 */
const SEEN_CAP = 20;

/** `failed` is red; a run that kept partial state is amber and not a break. */
export type FaultSeverity = "error" | "warning";

export type AgentFault = Readonly<{
  item: AiActivityItem;
  severity: FaultSeverity;
}>;

function severityOf(state: string): FaultSeverity | null {
  if (state === "failed") {
    return "error";
  }
  return state === "degraded" ? "warning" : null;
}

/**
 * The acknowledgements this browser holds.
 *
 * Storage can refuse both halves of this, and a refusal is not an error worth
 * reporting to anybody: private-mode and locked-down profiles throw on access,
 * and what the reader gets then is a fault that raises once per load, which is
 * the safe direction to fail in. The refusal is caught where it happens and
 * turned into the empty answer rather than left to take the rail down.
 */
function readSeen(): readonly string[] {
  try {
    const raw = window.localStorage.getItem(SEEN_KEY);
    if (raw === null) {
      return [];
    }
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed)
      ? parsed.filter((id): id is string => typeof id === "string")
      : [];
  } catch (refusedOrCorrupt) {
    // Both cases end here on purpose: storage this browser will not hand over,
    // and a value some earlier build wrote in another shape. Neither is
    // recoverable and both mean the same thing, which is that nothing is known
    // to have been seen yet.
    void refusedOrCorrupt;
    return [];
  }
}

function writeSeen(ids: readonly string[]): void {
  try {
    window.localStorage.setItem(SEEN_KEY, JSON.stringify(ids));
  } catch (refused) {
    // A browser that will not store the mark still gets the fault raised, which
    // is the behaviour this whole module exists to guarantee. Nothing else in
    // the rail depends on the write landing.
    void refused;
  }
}

export type AgentFaultReading = Readonly<{
  /** The newest broken run this reader has not been shown, or null. */
  fault: AgentFault | null;
  /** Mark the current fault as delivered. The panel opening calls this. */
  acknowledge: () => void;
}>;

/**
 * The fault standing over today's runs, and the way to clear it.
 *
 * It reads the feed's own `faults` arm and not `recent`, and the difference is
 * the whole of what this hook promises. `recent` is the newest ten occurrences
 * of ANY outcome, so ten later successes push a fault off it — and a fault held
 * until somebody acknowledges it would then be released with nobody having
 * looked, which is exactly the run that failed at four in the morning. The arm
 * is bounded on faults alone, so only another fault can displace one.
 *
 * `stalled` is not here and is not an omission: it is derived at read time for a
 * live run past its own lease, so it lives in `running` and clears itself when
 * the run settles. Nobody has to acknowledge a stall.
 */
export function useAgentFault(
  faults: readonly AiActivityItem[],
): AgentFaultReading {
  const [seen, setSeen] = useState<readonly string[]>(readSeen);

  // EVERY unacknowledged fault the feed carries, not just the newest. What the
  // orb shows is the first of them, but what the panel DELIVERS is all of them:
  // it lists every settled run. Acknowledging only the one the orb happened to
  // be showing would leave the colour up after the reader had already been told
  // about both, and clear one more fault per open — which is the opposite of
  // the rule this module exists to keep.
  const unacknowledged = useMemo(
    () =>
      faults.flatMap((item) => {
        const severity = severityOf(item.state);
        return severity === null || seen.includes(item.id)
          ? []
          : [{ item, severity }];
      }),
    [faults, seen],
  );

  const acknowledge = useCallback(() => {
    if (unacknowledged.length === 0) {
      return;
    }
    setSeen((current) => {
      const next = [
        ...unacknowledged.map((entry) => entry.item.id),
        ...current,
      ].slice(0, SEEN_CAP);
      writeSeen(next);
      return next;
    });
  }, [unacknowledged]);

  // The orb carries one state, so it carries the FIRST fault: the faults arm
  // is newest-first, and the newest break is the one worth colouring for.
  return { fault: unacknowledged[0] ?? null, acknowledge };
}
