// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// WHEN the machinery acted, across the window the receipt reports: one mark per
// line at the instant it happened, coloured by who acted, a bar where one job
// touched many records. The list under it is the same lines in words, so the
// strip is a picture of them and never the only place a fact is said.

import { type CSSProperties, useMemo } from "react";
import {
  formatDateTime,
  formatDayMonth,
  formatNumber,
  formatTimeOfDay,
  hourInZone,
} from "../format/format";
import { zoneOffsetMs } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { magicSentenceKey } from "./magic.keys";
import type { MagicLine, MagicReceipt } from "./magic.queries";
import "./magic.timeline.css";

// Agent work wears indigo because a model decided it; sync and rules are grey
// because nothing was inferred. Waiting is the staged ring, failure the one
// state colour.
const MARK_KINDS = ["agent", "sync", "waiting", "failed"] as const;
export type MarkKind = (typeof MARK_KINDS)[number];

const MARK_LABEL: Readonly<Record<MarkKind, MessageKey>> = {
  agent: "magic.timeline.agent",
  sync: "magic.timeline.sync",
  waiting: "magic.lane.needsYou",
  failed: "magic.lane.couldNotComplete",
};

const HOUR_MS = 60 * 60 * 1000;
const DAY_HOURS = 24;

// Collision is judged at a typical plot width rather than measured: a strip
// that re-laid itself on every resize would move marks under the pointer.
const NOMINAL_PLOT_PX = 640;
const DOT_PX = 10;
const BAR_PX = 6;
const PLOT_PX = 56;
const STACK_GAP_PX = 2;

// Tick steps a reader can count in, and the most that fit under the strip.
const TICK_STEP_HOURS = [1, 2, 3, 6, 12, 24, 48, 168];
const MAX_TICKS = 7;
// A tick this close to an end would print over the start label, which is a
// date or a time, or over the shorter "Now".
const START_CLEAR = 0.12;
const END_CLEAR = 0.08;

export type TimelineMark = Readonly<{
  line: MagicLine;
  kind: MarkKind;
  at: number;
  // Pixels right of `at`, for a bar set beside one already standing there.
  nudge: number;
  bottom: number;
  height: number;
}>;

// A watching line is dated when the condition was OBSERVED, so placing it
// would date every outage to this page load; its own section says since when.
export function markKind(line: MagicLine): MarkKind | null {
  switch (line.lane) {
    case "needs_you":
      return "waiting";
    case "could_not_complete":
      return "failed";
    case "watching":
      return null;
    case "done":
      return line.actor.type === "agent" ? "agent" : "sync";
  }
}

// Height grows with the log of the records a job touched: 1,200 filed emails
// must read taller than 5 without flattening every single change to nothing.
function markHeight(count: number): number {
  if (count <= 1) {
    return DOT_PX;
  }
  return Math.min(PLOT_PX, Math.round(DOT_PX + 9 * Math.log10(count)));
}

/**
 * Every line placed on the strip: across by when it happened, and a dot up
 * where an earlier mark already stands there, so two a minute apart both show.
 */
export function placeMarks(receipt: MagicReceipt): readonly TimelineMark[] {
  const start = Date.parse(receipt.since);
  const span = Date.parse(receipt.as_of) - start;
  if (!(span > 0)) {
    return [];
  }
  const near = DOT_PX / NOMINAL_PLOT_PX;
  // A lane off a payload this client cannot read places nothing rather than
  // throwing: the summary above already says it may be incomplete.
  const lines = [receipt.needs_you, receipt.could_not_complete, receipt.done]
    .flatMap((lane) => (Array.isArray(lane) ? lane : []))
    .sort((a, b) => Date.parse(a.occurred_at) - Date.parse(b.occurred_at));
  const placed: TimelineMark[] = [];
  for (const line of lines) {
    const kind = markKind(line);
    if (!kind) {
      continue;
    }
    const offset = (Date.parse(line.occurred_at) - start) / span;
    const at = Math.min(1, Math.max(0, offset));
    const height = markHeight(line.count ?? 1);
    const under = placed.filter((mark) => Math.abs(mark.at - at) < near);
    placed.push({ line, kind, at, height, ...standing(under, height) });
  }
  return placed;
}

// A bar stands on the baseline, where its height is read, beside whatever
// already stands there, dot or bar; a dot climbs onto whatever stands where it
// falls.
function standing(
  under: readonly TimelineMark[],
  height: number,
): { nudge: number; bottom: number } {
  if (height > DOT_PX) {
    const nudge = under
      .filter((mark) => mark.bottom === 0)
      .reduce(
        (right, mark) =>
          Math.max(
            right,
            mark.nudge + widthOf(mark) / 2 + STACK_GAP_PX + BAR_PX / 2,
          ),
        0,
      );
    return { nudge, bottom: 0 };
  }
  const top = under.reduce(
    (most, mark) => Math.max(most, mark.bottom + mark.height),
    0,
  );
  return {
    nudge: 0,
    bottom: top === 0 ? 0 : Math.min(top + STACK_GAP_PX, PLOT_PX - height),
  };
}

function widthOf(mark: TimelineMark): number {
  return mark.height > DOT_PX ? BAR_PX : DOT_PX;
}

export type AxisTick = Readonly<{ at: number; iso: string; day: boolean }>;

/**
 * The instants the axis names: whole hours on the reader's clock for a window
 * under a few days, midnights beyond that, never more than fit. A midnight
 * names its day either way, so a window across two nights says which is which.
 */
export function axisTicks(
  since: string,
  asOf: string,
  zone: string,
): readonly AxisTick[] {
  const start = Date.parse(since);
  const span = Date.parse(asOf) - start;
  if (!(span > 0)) {
    return [];
  }
  const step =
    TICK_STEP_HOURS.find((hours) => span / HOUR_MS / hours <= MAX_TICKS) ??
    TICK_STEP_HOURS[TICK_STEP_HOURS.length - 1];
  const ticks: AxisTick[] = [];
  let midnights = 0;
  for (
    let instant = wholeHourFrom(start, zone);
    instant < start + span;
    instant = wholeHourFrom(instant + 1, zone)
  ) {
    const hour = hourInZone(new Date(instant), zone);
    const at = (instant - start) / span;
    if (hour === 0) {
      midnights += 1;
    }
    // Days step from the first midnight in the window, so "every other day"
    // never skips the one the reader was away on.
    const onStep =
      step >= DAY_HOURS
        ? hour === 0 && (midnights - 1) % (step / DAY_HOURS) === 0
        : hour % step === 0;
    if (onStep && at > START_CLEAR && at < 1 - END_CLEAR) {
      ticks.push({
        at,
        iso: new Date(instant).toISOString(),
        day: hour === 0,
      });
    }
  }
  return ticks;
}

// The first whole hour on the zone's clock at or after `ms`. A whole UTC hour
// is not one in a zone set off by a half or three quarters of an hour, where
// it would print "00:45" under a tick drawn as midnight.
function wholeHourFrom(ms: number, zone: string): number {
  const offset = zoneOffsetMs(ms, zone) % HOUR_MS;
  const shift = (offset + HOUR_MS) % HOUR_MS;
  return Math.ceil((ms + shift) / HOUR_MS) * HOUR_MS - shift;
}

// Past a day "03:00" alone would not say which day, so the axis, and a line's
// own time beside the list, name the day instead.
export function spansDays(since: string, asOf: string): boolean {
  return Date.parse(asOf) - Date.parse(since) > DAY_HOURS * HOUR_MS;
}

export function axisLabel(
  iso: string,
  day: boolean,
  locale: Locale,
  zone: string,
): string {
  return day
    ? formatDayMonth(iso, locale, zone)
    : formatTimeOfDay(iso, locale, zone);
}

function markWhen(
  iso: string,
  day: boolean,
  locale: Locale,
  zone: string,
): string {
  return day
    ? formatDateTime(iso, locale, zone)
    : formatTimeOfDay(iso, locale, zone);
}

type MarkVars = CSSProperties & Record<`--${string}`, string>;

function share(at: number): string {
  return `${(at * 100).toFixed(2)}%`;
}

export function MagicTimeline({
  receipt,
  zone,
}: Readonly<{ receipt: MagicReceipt; zone: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const marks = useMemo(() => placeMarks(receipt), [receipt]);
  const ticks = useMemo(
    () => axisTicks(receipt.since, receipt.as_of, zone),
    [receipt.since, receipt.as_of, zone],
  );
  if (marks.length === 0) {
    return null;
  }
  const days = spansDays(receipt.since, receipt.as_of);
  const from = axisLabel(receipt.since, days, locale, zone);
  const drawn = MARK_KINDS.filter((kind) =>
    marks.some((mark) => mark.kind === kind),
  );
  const markTitle = (mark: TimelineMark): string => {
    const sentence = magicSentenceKey(mark.line.summary.key);
    const count = mark.line.count ?? 1;
    return [
      sentence
        ? t(sentence, mark.line.summary.values)
        : t(MARK_LABEL[mark.kind]),
      count > 1 || mark.line.count_is_floor === true
        ? plural(
            mark.line.count_is_floor === true
              ? "magic.aboutCountAtLeast"
              : "magic.aboutCount",
            count,
            { count: formatNumber(count, locale) },
          )
        : null,
      markWhen(mark.line.occurred_at, days, locale, zone),
    ]
      .filter(Boolean)
      .join(" · ");
  };
  return (
    <figure className="magic-timeline">
      <figcaption className="magic-timeline-head t-caption">
        <span>{t("magic.timeline.title")}</span>
        <ul className="magic-timeline-legend">
          {drawn.map((kind) => (
            <li key={kind}>
              <span
                className="magic-mark"
                data-kind={kind}
                aria-hidden="true"
              />
              {t(MARK_LABEL[kind])}
            </li>
          ))}
        </ul>
      </figcaption>
      <div
        className="magic-timeline-plot"
        role="img"
        aria-label={t("magic.timeline.summary", { from })}
      >
        {marks.map((mark, index) => {
          const vars: MarkVars = {
            "--at": share(mark.at),
            "--nudge": `${mark.nudge}px`,
            "--bottom": `${mark.bottom}px`,
            "--height": `${mark.height}px`,
            "--order": String(index),
          };
          return (
            <span
              key={`${mark.line.lane}-${mark.line.id}`}
              className="magic-mark"
              data-kind={mark.kind}
              data-shape={mark.height > DOT_PX ? "bar" : "dot"}
              style={vars}
              title={markTitle(mark)}
            />
          );
        })}
        <span className="magic-timeline-now" />
      </div>
      <div className="magic-timeline-axis t-caption" aria-hidden="true">
        <span className="magic-timeline-start">{from}</span>
        {ticks.map((tick) => {
          const vars: MarkVars = { "--at": share(tick.at) };
          return (
            <span key={tick.iso} className="magic-timeline-tick" style={vars}>
              {axisLabel(tick.iso, tick.day, locale, zone)}
            </span>
          );
        })}
        <span className="magic-timeline-end">{t("magic.timeline.now")}</span>
      </div>
    </figure>
  );
}
