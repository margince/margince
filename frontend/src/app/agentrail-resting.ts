// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import { formatNumber } from "../format/format";
import type { Locale, PluralTranslator, Translator } from "../i18n";
import { IDLE_ORDER, type IdleKind } from "./agentrail-copy";
import { plain, type SpokenLine } from "./ai-activity-speak";

// What the agent section says while it is at rest, and how long each thing gets
// to say it.
//
// Its own file because the resting line answers a different question from every
// other line the section draws. A fault, a run in flight and a read this tab
// just made all report ONE thing that is happening; this reports what is true
// when nothing is, which is a selection problem rather than a reporting one —
// which of several standing facts to say, and in what order, and for how long.
//
// TWO RULES SHAPE IT, and the first is the older one.
//
// It never invents activity. Every reading here is something the installation
// was asked and answered, and a reading with nothing to report is ABSENT rather
// than reworded into a cheerful nothing. Nor does it fill a quiet moment with
// standing advice: help lives where a reader asks for it, not in this rotation.
//
// It must not go stale. That is the newer one, and it is what this file was
// pulled out of `agentrail.tsx` to fix: the rotation carried whichever run
// settled most recently, forever, so an installation that summarised one contact
// at nine in the morning spent the rest of the working day with "What I know
// about Sabine Mayer is ready." in the corner of every screen. A settled run
// ages out of the rotation (`STILL_NEWS_MS`) instead.

/**
 * How long a run that has finished stays worth saying.
 *
 * The feed's `recent` is bounded to LOCAL MIDNIGHT, which is the right bound for
 * a recap — "what got done today" is a day's question — and much too wide for a
 * status line. A brief that finished at seven is news to the reader who arrives
 * at nine and furniture to the one still at their desk at six, and the rail
 * cannot tell those two apart except by the clock.
 *
 * Three hours is the compromise, and it is chosen for the morning brief because
 * that is the run whose value most outlives it: long enough that a reader
 * arriving at the start of their day is told what the night produced, short
 * enough that the line is never announcing breakfast after lunch.
 */
const STILL_NEWS_MS = 3 * 60 * 60_000;

/**
 * How many settled runs the rotation carries at once.
 *
 * The feed serves up to ten. All ten would make one pass of the rotation over a
 * minute long, which is the same failure as a pinned line wearing a different
 * face: a reader glancing twice at the rail would see two unrelated sentences
 * and no sense of what it is for. Three is what a reader can hold as "what the
 * agent got done just now".
 */
const SETTLED_DEPTH = 3;

/** How long a reading holds the line. */
const READING_HOLD_MS = 6_000;

/**
 * An occurrence, as far as the freshness question is concerned.
 *
 * Structural rather than the contract's own type: this file asks one thing of an
 * item and asking for the whole `AiActivityItem` would make the pure function
 * testable only by building one.
 */
type Settled = Readonly<{
  started_at: string;
  finished_at?: string | null;
}>;

/**
 * The runs that settled recently enough to still be news, newest first.
 *
 * `now` is passed rather than read, so this is a function of its arguments and a
 * test of it is a test rather than a race with the clock.
 *
 * It ages on `finished_at` and falls back to `started_at`, which is the
 * conservative direction and deliberately so: a run started before it finished,
 * so the fallback can only age an item out EARLIER than the truth, never keep a
 * stale one. Dropping an item that carries no finish time would be the other
 * way round — a settled run the rail silently refuses to mention — and the one
 * failure this selection must not have is under-reporting, because nothing on
 * screen distinguishes "nothing finished" from "something finished and this
 * function could not date it".
 */
export function stillNews<T extends Settled>(
  items: readonly T[],
  now: number,
): readonly T[] {
  return items
    .filter((item) => {
      const at = Date.parse(item.finished_at ?? item.started_at);
      return Number.isFinite(at) && now - at < STILL_NEWS_MS;
    })
    .slice(0, SETTLED_DEPTH);
}

/**
 * The words the rail's line is said in.
 *
 * A bundle rather than a bare translator because one of them is a COUNT, and a
 * count is not a lookup: "1 decisions waiting" is the tell of a surface that
 * pastes a number onto a noun, so the queue's line goes through the reader's
 * own plural rule and its number through their own grouping.
 */
export type RailWords = Readonly<{
  t: Translator;
  waiting: (count: number) => string;
}>;

/** The bundle, built once per render from the reader's locale. */
export function railWords(
  t: Translator,
  plural: PluralTranslator,
  locale: Locale,
): RailWords {
  return {
    t,
    waiting: (count) =>
      plural("agent.line.waiting", count, {
        count: formatNumber(count, locale),
      }),
  };
}

/** What the section has read, in the shape the resting line needs it. */
export type RestingFacts = Readonly<{
  /** Decisions staged for this human; undefined until the read answers. */
  waiting: number | undefined;
  /** The connections read failed, so the all-clear would be a guess. */
  sourcesUnknown: boolean;
  /**
   * The line saying this deployment invents every answer it gives, or null when
   * it does not. The SENTENCE rather than a flag beside it: two fields would be
   * a state where the claim is made and the words for it are missing, and the
   * caller is the half that has a translator.
   */
  developmentLine: string | null;
  /** The runs still fresh enough to be news, already in the reader's words. */
  settled: readonly SpokenLine[];
}>;

/**
 * The true things a resting agent can say, in the order it says them.
 *
 * Every one is a reading it already made. A kind with nothing to report is
 * absent rather than reworded into a cheerful nothing, so an installation with a
 * clean queue and no fresh runs rotates through one line and not three.
 *
 * Never empty: an installation with nothing at all to report says so, and that
 * sentence is a reading like any other — it is what a clean queue and a reachable
 * agent actually add up to.
 */
export function restingReadings(
  facts: RestingFacts,
  words: RailWords,
): readonly SpokenLine[] {
  const said: Partial<Record<IdleKind, readonly SpokenLine[]>> = {
    // What the scheduled runner finished while nobody was looking, newest
    // first and only while it is still news (`stillNews`). Several of them,
    // because a day settles several runs and the newest being the only one
    // ever said is both less than the agent did and, by the afternoon, older
    // than the reader.
    sources: facts.sourcesUnknown
      ? [plain(words.t("agent.line.sourcesUnknown"))]
      : undefined,
    finished: facts.settled,
    waiting:
      facts.waiting !== undefined && facts.waiting > 0
        ? [plain(words.waiting(facts.waiting))]
        : undefined,
    // The development path answers every call with an invention, and a reader who
    // does not know that is being misled by a product that looks like it works.
    model:
      facts.developmentLine === null
        ? undefined
        : [plain(facts.developmentLine)],
  };
  const lines = IDLE_ORDER.flatMap((kind) => said[kind] ?? []);
  // Never an all-clear while the source check failed: that reading is in
  // `lines` above, so the fallback is reached only when every read answered.
  return lines.length === 0 ? [plain(words.t("agent.line.allClear"))] : lines;
}

/**
 * Which line is showing, changing on its own.
 *
 * Slow: this sits at the edge of every screen all day, and a line that changed
 * every second would be movement in the corner of somebody's eye while they were
 * trying to read something else. One line, long enough to read twice.
 *
 * A chained timeout rather than an interval, so each step arms the next and a
 * reading that changes restarts its own hold.
 */
export function useRestingLine(readings: readonly SpokenLine[]): SpokenLine {
  const [tick, setTick] = useState(0);
  const span = readings.length;
  const at = span === 0 ? 0 : tick % span;
  useEffect(() => {
    if (span < 2) {
      return;
    }
    const timer = setTimeout(() => setTick(tick + 1), READING_HOLD_MS);
    return () => clearTimeout(timer);
    // The step it schedules is written out rather than taken from an updater,
    // which is what makes `tick` a real dependency: each step arms the next, so
    // the effect HAS to run again when one lands, and an updater would hide
    // that from the dependency list and leave the rotation stopped after one
    // move. The closure cannot go stale for the same reason — the effect is
    // torn down and re-armed by the change it is reading.
  }, [span, tick]);
  // The list can shorten under it when a read answers, so the index is clamped
  // rather than trusted.
  const reading = readings[at] ?? readings[0];
  if (reading === undefined) {
    throw new Error("useRestingLine needs at least one reading");
  }
  return reading;
}
