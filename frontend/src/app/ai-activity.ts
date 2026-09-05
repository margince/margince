// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { QueryClient } from "@tanstack/react-query";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { api } from "../api/client";
import { useModelCallsInFlight } from "../api/model-inflight";
import type { components } from "../api/schema";
import { throwProblem } from "../screens/common";
import { displayedKinds } from "./ai-activity-lines";

type AiActivityItem = components["schemas"]["AiActivityItem"];

/** Fast while the AI is mid-occurrence; slow otherwise. */
const POLL_LIVE_MS = 3_000;
const POLL_IDLE_MS = 30_000;

/**
 * How long this tab's own ask stays reported after its request ended.
 *
 * A model call that answers in a tenth of a second is still the agent doing
 * something a person asked for, and a light that appears and disappears inside
 * one animation frame is a light nobody sees. This is the same floor the
 * ticker's lines are given (`LINGER_MS` in agentrail-ticker.ts) and for the
 * same reason: long enough to register, short enough that the report is never
 * stale. It stretches the PRESENTATION of a real event and invents none: the
 * offline fake model answers in about a tenth of a second, so without a floor
 * every dev stack and every demo would show exactly the nothing this change
 * exists to fix.
 */
const ASK_LINGER_MS = 900;

/**
 * How long the feed is watched at the live cadence for a run this tab started.
 *
 * A durable run is not in the feed when its own request answers, and cannot be:
 * the 202 means the row is committed, and the occurrence reaches the projection
 * through the outbox — the relay's own poll, then the bus, then the consumer's
 * upsert. So the read fired on the 202 asks at the one instant the answer is
 * guaranteed to be no, and with nothing running the cadence then drops to
 * POLL_IDLE_MS. That is how a website read a rep pressed and then watched
 * announced itself on the rail up to half a minute later, or — for a reading
 * that settled inside the window — never.
 *
 * The ceiling is the idle period itself, because past it the ordinary cadence
 * has asked anyway: watching longer would not be watching, it would be polling
 * fast for a run that is not coming. It ends on the clock rather than on the
 * feed carrying the run, because from that moment the live rule below is what
 * holds the cadence — the watch only has to cover the gap before it.
 */
const START_WATCH_MS = POLL_IDLE_MS;

const ACTIVITY_KEY = ["me", "ai-activity"] as const;

/**
 * How many durable runs this tab has started. A counter and not a flag: two
 * presses are two waits, and the second must re-arm the first's rather than
 * expire on its clock.
 */
let starts = 0;
const startListeners = new Set<() => void>();

function startCount(): number {
  return starts;
}

function subscribeToStarts(onChange: () => void): () => void {
  startListeners.add(onChange);
  return () => {
    startListeners.delete(onChange);
  };
}

/**
 * Report that this tab just started a durable run, so the feed goes looking for
 * it instead of waiting for it.
 *
 * For the surfaces that START a run and answer 202 — a website read from a
 * company page, a document reading from an attachment — rather than hold a
 * model call open: those never count as an ask in flight, so nothing else here
 * knows the agent has been given work. The immediate read is kept for the case
 * it can actually answer — a request that JOINED a crawl already in flight,
 * whose occurrence has been in the feed for minutes — and the watch covers the
 * one it cannot.
 */
export function watchStartedAiRun(client: QueryClient): void {
  starts += 1;
  for (const listener of startListeners) {
    listener();
  }
  void client.refetchQueries({ queryKey: ACTIVITY_KEY });
}

/** One frozen empty list, so an absent read never mints a new array identity. */
const NOTHING: readonly AiActivityItem[] = Object.freeze([]);

export type AiActivity = Readonly<{
  /** Occurrences that are queued, running, or live past their lease; empty while the read is absent. */
  running: readonly AiActivityItem[];
  /** Occurrences that settled since local midnight, newest first. */
  recent: readonly AiActivityItem[];
  /**
   * What went wrong today — failed and degraded runs, newest first.
   *
   * Beside `recent` rather than filtered out of it, and the difference is the
   * whole reason it exists: `recent` carries the newest ten occurrences of any
   * outcome, so ten later successes push a fault out of it. A fault held until
   * somebody acknowledges it would then be released with nobody having looked,
   * which is exactly the overnight run that failed while its owner was asleep.
   */
  faults: readonly AiActivityItem[];
  /** Whether any AI work is live RIGHT NOW, as reported by a read that answered. */
  working: boolean;
  /**
   * Whether THIS TAB is holding a request open to a route that calls a model.
   *
   * Its own fact, beside the feed rather than mixed into it, because the two
   * know different things. The feed knows what the work IS and is the only
   * thing that may say so; it arrives on a poll, so it cannot answer the
   * instant somebody presses a button. This answers exactly that instant and
   * nothing else: no kind, no state, no sentence.
   */
  asking: boolean;
}>;

/**
 * What the AI is doing for this person, polled while somebody is looking.
 *
 * ONE read over one projection: a scheduled run and a document reading arrive
 * through the same feed, so a new kind of AI work never adds a call here.
 *
 * The rail's doctrine applies to `working`: a read that has not answered, or
 * that this seat may not make, is ABSENT rather than a zero. So `working` is
 * false for a pending read and false for a failed one, and true only when a
 * body came back carrying a live run — nothing here lets `undefined` flicker
 * through as a standing the reader would take for "at rest".
 */
export function useAiActivity(): AiActivity {
  const client = useQueryClient();
  const [visible, setVisible] = useState(() => !document.hidden);
  const open = useModelCallsInFlight();
  const watching = useStartedRunWatch();

  useEffect(() => {
    const onVisibilityChange = () => {
      setVisible(!document.hidden);
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
    };
  }, []);

  const query = useQuery({
    queryKey: ACTIVITY_KEY,
    enabled: visible,
    queryFn: async () => {
      // The rail asks for the kinds it draws. Without this the server's bounds
      // fall on the complete record and can spend all ten `recent` slots on
      // work this surface renders nothing for.
      //
      // An empty list would FAIL OPEN, not closed, which is why it is refused
      // here rather than sent: openapi-fetch's query serializer drops a
      // zero-length array entirely, so the request would carry no `kinds` at
      // all and the server would answer with the complete record — the exact
      // failure this filter exists to prevent, and the server's own minItems /
      // 422 guard would never see it.
      const kinds = displayedKinds();
      if (kinds.length === 0) {
        throw new Error(
          "the rail narrates no kinds, so asking the server for them would silently ask for all of them",
        );
      }
      const { data, error } = await api.GET("/me/ai-activity", {
        params: { query: { kinds } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    // Coalesced through NOTHING for the same reason the result below is: the
    // cached body is whatever the server sent, and reaching into it for a
    // length is what turns an off-contract 200 into a throw.
    // An ask in flight counts as live even before the feed agrees. The
    // projection is written from an event the router publishes, so there is a
    // short window in which this tab knows the agent is working and the feed
    // does not yet, and resting at the idle cadence through that window is how
    // a run that lasted five seconds was read at thirty-second resolution.
    // The raw count and not the lingering flag: the linger is presentation,
    // and the cadence follows the fact.
    //
    // A run this tab started is the third thing that makes the cadence live,
    // and the only one neither of the other two can see: the feed does not
    // carry it yet and no model call is open for it.
    refetchInterval: (q) =>
      watching || open > 0 || (q.state.data?.running ?? NOTHING).length > 0
        ? POLL_LIVE_MS
        : POLL_IDLE_MS,
  });

  // Both EDGES of an ask, read immediately rather than waited for: the request
  // leaving is when the occurrence appears, and the request answering is when
  // it settles. Polling alone would show each of those up to a poll late, which
  // for the short tasks a person triggers and then watches is most of the run.
  //
  // On `open` rather than on the lingering flag above, because the linger is a
  // presentation floor and this is a read: waiting it out would put the settled
  // line on screen most of a second after the draft it describes.
  const previousOpen = useRef(open);
  useEffect(() => {
    if (open === previousOpen.current) {
      return;
    }
    previousOpen.current = open;
    void client.refetchQueries({ queryKey: ACTIVITY_KEY });
  }, [open, client]);

  // The app disables focus refetching for every query (FE-PARAM-3), so
  // returning to the tab is otherwise answered out of the cache — and the
  // cached body is the one that is wrong, because the run it shows as live is
  // the run that finished while the tab was away. Hence an explicit refetch.
  //
  // It sits BELOW the query and not in the visibility listener because both
  // placements are silently inert: refetchQueries skips a query it still
  // considers disabled, and the enabled flag above is applied by the query's
  // own effect — so the ask has to be queued after that effect has run.
  const wasVisible = useRef(visible);
  useEffect(() => {
    const returned = visible && !wasVisible.current;
    wasVisible.current = visible;
    if (returned) {
      void client.refetchQueries({ queryKey: ACTIVITY_KEY });
    }
  }, [visible, client]);

  // Coalesced ONCE, and `working` is read off the coalesced list rather than
  // off the body: a 200 whose shape is not the one the contract promises is
  // another absent read, and reaching into it for a length is how the rail's
  // whole section throws instead of reporting nothing.
  const answered = query.data;
  const running = answered?.running ?? NOTHING;
  const recent = answered?.recent ?? NOTHING;
  const faults = answered?.faults ?? NOTHING;
  const asking = useLingeringAsk(open, recent[0]?.id);
  return {
    running,
    recent,
    faults,
    // STALLED is not working. The server derives that state for an occurrence
    // whose own source says it should have finished by now, and the chrome that
    // reads `working` pulses to say the AI is busy — so counting a stalled item
    // here would animate "still going" over a line that reads "it may have
    // stopped", softening the one verdict this state exists to deliver.
    working: running.some((item) => item.state !== "stalled"),
    asking,
  };
}

/**
 * Whether this tab is still looking for a run it started.
 *
 * The start is module state rather than a prop for the same reason the
 * in-flight count is (`api/model-inflight.ts`): the surface that presses the
 * button and the rail that reports the work are on opposite sides of the app,
 * and threading a callback between them would put the rail's cadence in the
 * hands of every screen that starts anything.
 *
 * The count seen at mount is not a start this tab is waiting on — it happened
 * before this rail existed — so the ref opens on it rather than on zero, and a
 * remounted rail does not arm a watch nobody asked for.
 */
function useStartedRunWatch(): boolean {
  const started = useSyncExternalStore(
    subscribeToStarts,
    startCount,
    startCount,
  );
  const [watching, setWatching] = useState(false);
  const seen = useRef(started);
  useEffect(() => {
    if (started === seen.current) {
      return undefined;
    }
    seen.current = started;
    setWatching(true);
    const timer = setTimeout(() => {
      setWatching(false);
    }, START_WATCH_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [started]);
  return watching;
}

/**
 * An ask, held on screen for long enough to be seen.
 *
 * `open` is the truth and this is what a reader can perceive of it. The floor
 * lives here rather than in the store so the count stays honest for anybody
 * else who reads it: a store that lied about what was in flight to make a light
 * look right would hand its next consumer a fact that is not one.
 *
 * The floor is for a call the feed never caught up with. `newestSettled` is
 * the feed's newest settled occurrence; a DIFFERENT one arriving while the ask
 * lingers is the feed naming the work that just finished, and the linger
 * yields to it at once — holding a bare "working" over a line that already
 * says what was done would be the presentation contradicting the record.
 */
function useLingeringAsk(
  open: number,
  newestSettled: string | undefined,
): boolean {
  const [lingering, setLingering] = useState(false);
  // What the feed's newest settled occurrence was when the ask left. Null
  // while no ask is being held, so a settlement from some other run does not
  // count as this one's.
  const settledAtAsk = useRef<{ id: string | undefined } | null>(null);
  useEffect(() => {
    if (open > 0) {
      settledAtAsk.current ??= { id: newestSettled };
      setLingering(true);
      return undefined;
    }
    if (!lingering) {
      // Nothing is being held over, so there is nothing to stop holding. The
      // guard is what keeps a rail that has never seen an ask from arming a
      // timer on every mount, which is a page that never goes quiet.
      settledAtAsk.current = null;
      return undefined;
    }
    if (settledAtAsk.current && settledAtAsk.current.id !== newestSettled) {
      setLingering(false);
      return undefined;
    }
    const timer = setTimeout(() => {
      setLingering(false);
    }, ASK_LINGER_MS);
    return () => {
      clearTimeout(timer);
    };
  }, [open, lingering, newestSettled]);
  return open > 0 || lingering;
}
