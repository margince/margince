// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useMemo, useState } from "react";
import type { components } from "../api/schema";
import { readStoredJson, STORAGE_KEYS, writeStored } from "./storage";

type AiActivityItem = components["schemas"]["AiActivityItem"];

/**
 * The run that broke, held on the orb until somebody has actually looked at it.
 *
 * A scheduled run fails at four in the morning and the contact it ran for is
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
 * The seen marks are per browser, so a reload or a second screen does not
 * re-raise a fault the reader already dealt with.
 */
function readSeen(): readonly string[] {
  return readStoredJson(STORAGE_KEYS.faultsSeen, seenIds) ?? [];
}

function seenIds(value: unknown): readonly string[] | null {
  return Array.isArray(value)
    ? value.filter((id): id is string => typeof id === "string")
    : null;
}

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
      writeStored(STORAGE_KEYS.faultsSeen, JSON.stringify(next));
      return next;
    });
  }, [unacknowledged]);

  // The orb carries one state, so it carries the FIRST fault: the faults arm
  // is newest-first, and the newest break is the one worth colouring for.
  return { fault: unacknowledged[0] ?? null, acknowledge };
}
