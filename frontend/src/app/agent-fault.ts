// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { components } from "../api/schema";

type AiActivityItem = components["schemas"]["AiActivityItem"];
type ActivityKind = AiActivityItem["kind"];

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
 * How many acknowledgements are kept. Faults are rare and `recent` is bounded to
 * one local day, so a short list outlives every id that could still be raised,
 * and the oldest falling off can only ever re-raise a run from a previous day
 * that the feed no longer carries.
 */
const SEEN_CAP = 20;

/**
 * The kinds a person asks for and then waits on, where the surface that asked
 * reports the outcome itself: the composer shows the draft that came back or
 * the floor it fell to, the company page shows the brief, the fit, the answer,
 * the read.
 *
 * A fault in one of these is DELIVERED the moment it happens, on the screen the
 * reader is looking at, so the orb has nothing left to hold it for. It still
 * colours — a reader glancing at the corner should see that the ask broke — but
 * for FLASH_MS and no longer, after which it counts as seen and stays in the
 * panel's list of what went wrong today. Holding it until the panel was opened
 * kept the orb red through the whole afternoon over a draft the reader had
 * already retried and sent.
 *
 * Everything not named here is the scheduled and background work — the brief,
 * the sweep, the review, the document reading — that fails while nobody is
 * looking and has no other screen to land on. Those hold until acknowledged,
 * which is the rule this module was built for.
 *
 * Typed against the contract's own kinds, so a renamed task fails the build
 * here rather than quietly falling into the held-until-seen half.
 */
const ATTENDED: ReadonlySet<ActivityKind> = new Set<ActivityKind>([
  "summarize",
  "draft_reply",
  "offer_draft",
  "growth_fit",
  "corpus_ask",
  "cold_start",
  "site_extract",
]);

/**
 * How long an attended fault colours the orb before it counts as seen.
 *
 * Long enough to be noticed by a reader who looked away from the screen that
 * reported it, short enough that it is over before they have finished reading
 * the reason that screen gave them.
 */
const FLASH_MS = 8_000;

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
 * The fault standing over the settled runs, and the way to clear it.
 *
 * It reads `recent` and not `running`, because a run only reports `failed` or
 * `degraded` once it has settled, and a settled run is in `recent` by
 * construction. `stalled` is the other way round: it is derived at read time for
 * a live run past its own lease, so it lives in `running` and clears itself when
 * the run settles. Nobody has to acknowledge a stall, which is why it is not
 * here.
 */
export function useAgentFault(
  recent: readonly AiActivityItem[],
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
      recent.flatMap((item) => {
        const severity = severityOf(item.state);
        return severity === null || seen.includes(item.id)
          ? []
          : [{ item, severity }];
      }),
    [recent, seen],
  );

  const markSeen = useCallback((ids: readonly string[]) => {
    if (ids.length === 0) {
      return;
    }
    setSeen((current) => {
      const next = [...ids, ...current].slice(0, SEEN_CAP);
      writeSeen(next);
      return next;
    });
  }, []);

  const acknowledge = useCallback(() => {
    markSeen(unacknowledged.map((entry) => entry.item.id));
  }, [markSeen, unacknowledged]);

  // The orb carries one state, so it carries the FIRST fault: `recent` is
  // newest-first, and the newest break is the one worth colouring for.
  const fault = unacknowledged[0] ?? null;

  // EVERY attended fault times out on its own, not only the one the orb happens
  // to show. A scheduled fault that arrived later stands in front of an older
  // attended one, and a timer armed for the front of the list alone would leave
  // that attended fault waiting behind it — to flash, hours late, the moment
  // the scheduled one was acknowledged. Each attended fault gets its own clock
  // from the moment it is first seen, kept in a ref rather than re-armed on
  // every render, so a neighbour arriving does not restart a flash that was
  // already mostly over.
  const attended = unacknowledged
    .filter((entry) => ATTENDED.has(entry.item.kind))
    .map((entry) => entry.item.id);
  const flashes = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  useEffect(() => {
    const live = new Set(attended);
    for (const [id, timer] of flashes.current) {
      if (!live.has(id)) {
        clearTimeout(timer);
        flashes.current.delete(id);
      }
    }
    for (const id of attended) {
      if (!flashes.current.has(id)) {
        flashes.current.set(
          id,
          setTimeout(() => {
            flashes.current.delete(id);
            markSeen([id]);
          }, FLASH_MS),
        );
      }
    }
  }, [attended, markSeen]);
  // The last word belongs to the unmount: a timer left behind would mark a
  // fault seen on a rail that no longer exists.
  useEffect(() => {
    const armed = flashes.current;
    return () => {
      for (const timer of armed.values()) {
        clearTimeout(timer);
      }
      armed.clear();
    };
  }, []);

  return { fault, acknowledge };
}
