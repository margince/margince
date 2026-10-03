// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the machinery did while the reader was away.
//
// A RECEIPT AND A ROUTER, NEVER A SECOND INBOX, which is the endpoint's own
// words and the reason no row here carries a verb. A staged decision is decided
// where decisions are decided and an undo is the record's own; a row may point
// at the surface that holds the verb, and holding it here would put a second
// answer beside the first.
//
// FOUR LANES, because they ask different things: what is waiting on a human,
// what was promised and did not land, what an administrator must restore, and
// what already happened. One list would let a failure sort in beside a success,
// which is the one thing a receipt must never do.
//
// ONE LINE FOR THE CLEAR ONES. Every lane is answered in the summary at the
// top, each in its own words, and only a lane with lines in it draws a
// section: four headings over "nothing" said less than one line saying so.
//
// THE PICTURE BEFORE THE LIST. What got done is counted by kind and every line
// is placed on the window's clock, so a morning of 1,200 filed emails reads in
// one glance; the done lines themselves fold under that, one press away.
//
// A PLAIN PANEL, and Home's only receipt: the page hands in the changes that
// wait for a word and the night's digest, so "what happened while I was away"
// is answered in one place rather than three.

import type { LucideIcon } from "lucide-react";
import { CircleAlert, CircleCheck, CircleDashed, Sparkles } from "lucide-react";
import { type ReactNode, useState } from "react";
import { SegmentedControl } from "../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { floorFigure } from "../format/figure";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { MagicGlance } from "./magic.glance";
import { MagicLaneSection } from "./magic.lines";
import {
  fillsPage,
  MAGIC_WINDOWS,
  type MagicLane,
  type MagicLine,
  type MagicNotShown,
  type MagicReceipt,
  type MagicWindow,
  useMagic,
} from "./magic.queries";
import { MagicTimeline, spansDays } from "./magic.timeline";
import { sourceUnavailableText } from "./worklist.copy";
import "./brief.css";

// The reading order: what waits on you, what broke, what needs restoring, and
// what already happened. A list rather than four call sites, so a fifth lane is
// one entry.
const LANES = [
  "needs_you",
  "could_not_complete",
  "watching",
  "done",
] as const satisfies readonly MagicLane[];

const LANE_HEADING: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.lane.done",
  needs_you: "magic.lane.needsYou",
  could_not_complete: "magic.lane.couldNotComplete",
  watching: "magic.lane.watching",
};

// What there is none OF, per lane. Each says its own words because "nothing is
// waiting" and "nothing failed" are opposite news, and a shared "nothing here"
// would report them as one.
const LANE_CLEAR: Readonly<Record<MagicLane, MessageKey>> = {
  done: "magic.clear.done",
  needs_you: "magic.clear.needsYou",
  could_not_complete: "magic.clear.couldNotComplete",
  watching: "magic.clear.watching",
};

const LANE_COUNT: Readonly<Record<MagicLane, PluralBase>> = {
  done: "magic.count.done",
  needs_you: "magic.count.needsYou",
  could_not_complete: "magic.count.couldNotComplete",
  watching: "magic.count.watching",
};

// The tone a lane with lines in it wears. Waiting is indigo because what waits
// is a proposal an agent staged; a failure and an outage are outcomes.
const LANE_TONE: Readonly<Record<MagicLane, SummaryTone>> = {
  done: "success",
  needs_you: "ai",
  could_not_complete: "danger",
  watching: "warning",
};

const WINDOW_HEADING: Readonly<Record<MagicWindow, MessageKey>> = {
  brief: "magic.heading.brief",
  week: "magic.heading.week",
  month: "magic.heading.month",
};

const NOT_SHOWN_REASON: Readonly<Record<MagicNotShown["reason"], MessageKey>> =
  {
    unadmitted_action: "magic.notShown.unadmittedAction",
    unknown_entity_type: "magic.notShown.unknownEntityType",
    out_of_scope: "magic.notShown.outOfScope",
  };

type SummaryTone = "success" | "ai" | "danger" | "warning" | "neutral";

const TONE_ICON: Readonly<Record<SummaryTone, LucideIcon>> = {
  success: CircleCheck,
  ai: Sparkles,
  danger: CircleAlert,
  warning: CircleAlert,
  neutral: CircleDashed,
};

export function MagicPanel({
  lead,
  foot,
}: Readonly<{
  /** Home's changes that wait for a word, drawn above the lanes. */
  lead?: ReactNode;
  /** The night's digest, drawn under everything the window holds. */
  foot?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  // The READER's own zone. A receipt says when something happened to them, and
  // an instant rendered in UTC asks them to do the arithmetic.
  const zone = viewerZone();
  // How far back, chosen by the reader. The default is the server's own answer
  // — since the last brief — and the longer windows are for the reader who was
  // away, or who wants to see what an import set off.
  const [span, setSpan] = useState<MagicWindow>("brief");
  const magic = useMagic(span);
  const receipt = magic.data;
  const withheld = receipt?.sources_unavailable ?? [];
  const state = magic.isPending
    ? "loading"
    : magic.isError
      ? "failed"
      : "ready";
  // Lines only off a read that answered: a refresh that failed keeps the last
  // answer cached, and drawing it would offer undos under "did not load".
  const shown = state === "ready" ? receipt : undefined;
  const drawn = LANES.filter((lane) => hasLines(shown?.[lane]));
  const done = rowsOf(shown?.done);
  const doneRecords = recordsOf(done);
  const doneFloor = sumIsFloor("done", done);
  // "All" would claim the whole of a count that is only a floor.
  const doneFold = plural(
    doneFloor ? "magic.done.atLeast" : "magic.done.all",
    doneRecords,
    {
      count: floorFigure(
        formatNumber(doneRecords, locale),
        doneFloor,
        doneRecords,
      ),
    },
  );
  const byDay = shown ? spansDays(shown.since, shown.as_of) : false;
  return (
    <Panel
      className="magic-panel"
      title={t(WINDOW_HEADING[span])}
      titleAction={
        <SegmentedControl
          options={MAGIC_WINDOWS}
          value={span}
          onChange={setSpan}
          label={t("magic.window.label")}
          labels={{
            brief: t("magic.window.brief"),
            week: t("magic.window.week"),
            month: t("magic.window.month"),
          }}
        />
      }
    >
      <PanelBody>
        {/* WHICH WINDOW: "nothing happened" over an hour and over a day are
            different claims, and only the server knows which one this is. */}
        {shown?.since && (
          <PanelIntro>
            {t("magic.intro", {
              when: formatDateTime(shown.since, locale, zone),
            })}
          </PanelIntro>
        )}
        <SurfaceState
          state={state}
          emptyLabel=""
          loadingLabel={t("magic.loading")}
          detail={{ onRetry: () => void magic.refetch() }}
        >
          {/* The caveat BEFORE the summary. A reader who meets it after four
              answers has already read three of them as complete. */}
          <WithheldSources withheld={withheld} />
          <LaneSummary
            receipt={receipt}
            // "All clear" is forbidden while a lane could not be read: a lane
            // the reader may not see and a lane with nothing in it are
            // different answers, and only one of them is good news.
            canReportEmpty={withheld.length === 0}
          />
          <MagicGlance done={done} />
          {shown && <MagicTimeline receipt={shown} zone={zone} />}
        </SurfaceState>
      </PanelBody>
      {lead}
      {drawn.map((lane) => (
        <MagicLaneSection
          key={lane}
          lane={lane}
          title={t(LANE_HEADING[lane])}
          rows={shown?.[lane] ?? []}
          since={shown?.since}
          zone={zone}
          byDay={byDay}
          fold={lane === "done" ? doneFold : undefined}
        />
      ))}
      {(shown?.not_shown?.length ?? 0) > 0 && (
        <PanelBody>
          <NotShown entries={shown?.not_shown ?? []} />
        </PanelBody>
      )}
      {foot}
    </Panel>
  );
}

// A lane off a payload this client cannot read is not an empty one: absent is
// version skew, and only a list the server sent can say there is nothing in it.
function hasLines(rows: readonly MagicLine[] | undefined): boolean {
  return rowsOf(rows).length > 0;
}

function rowsOf(rows: readonly MagicLine[] | undefined): readonly MagicLine[] {
  return Array.isArray(rows) ? rows : [];
}

// How many records the lines stand for; a line without a count is one.
function byAnAgent(row: MagicLine): boolean {
  return row.actor.type === "agent";
}

function recordsOf(rows: readonly MagicLine[]): number {
  return rows.reduce((sum, row) => sum + (row.count ?? 1), 0);
}

// A sum is only a floor over a line whose read was cut short, or over a lane
// that filled its page and may hold more lines than it shows.
function sumIsFloor(lane: MagicLane, rows: readonly MagicLine[]): boolean {
  return (
    fillsPage(lane, rows) || rows.some((row) => row.count_is_floor === true)
  );
}

/**
 * Every lane answered on one line: how many lines it holds, or in its own words
 * that it holds none, or that this read cannot say.
 */
function LaneSummary({
  receipt,
  canReportEmpty,
}: Readonly<{ receipt: MagicReceipt | undefined; canReportEmpty: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  if (!receipt) {
    return null;
  }
  const answers = LANES.map((lane) => {
    const rows = receipt[lane];
    if (!Array.isArray(rows) || (rows.length === 0 && !canReportEmpty)) {
      return {
        lane,
        tone: "neutral" as const,
        text: t("magic.incomplete", { lane: t(LANE_HEADING[lane]) }),
      };
    }
    if (rows.length === 0) {
      return { lane, tone: "success" as const, text: t(LANE_CLEAR[lane]) };
    }
    // "Done for you" is what an agent did. Sync and rules keep records
    // current, which is maintenance, so they are counted apart rather than
    // inflating the headline.
    const agent = lane === "done" ? rows.filter(byAnAgent) : rows;
    const kept = lane === "done" ? rows.filter((row) => !byAnAgent(row)) : [];
    const figure = (base: PluralBase, of: readonly MagicLine[]) => {
      // Records on THIS PAGE, which is all the endpoint promises: one line may
      // stand for 1,200 filed emails, and the tiles and the fold count it so.
      const count = recordsOf(of);
      return plural(base, count, {
        count: floorFigure(
          formatNumber(count, locale),
          sumIsFloor(lane, of),
          count,
        ),
      });
    };
    const parts = [
      ...(agent.length > 0 || kept.length === 0
        ? [figure(LANE_COUNT[lane], agent)]
        : []),
      ...(kept.length > 0 ? [figure("magic.count.keptInSync", kept)] : []),
    ];
    return {
      lane,
      tone: agent.length > 0 ? LANE_TONE[lane] : ("neutral" as const),
      text: parts.join(" · "),
    };
  });
  return (
    <ul className="magic-summary" aria-label={t("magic.summary")}>
      {answers.map((answer) => (
        <SummaryItem key={answer.lane} tone={answer.tone} text={answer.text} />
      ))}
    </ul>
  );
}

function SummaryItem({
  tone,
  text,
}: Readonly<{ tone: SummaryTone; text: string }>) {
  const Icon = TONE_ICON[tone];
  return (
    <li className="magic-summary-item" data-tone={tone}>
      <Icon size={16} aria-hidden="true" />
      <span>{text}</span>
    </li>
  );
}

function WithheldSources({
  withheld,
}: Readonly<{ withheld: MagicReceipt["sources_unavailable"] }>) {
  const t = useT();
  if (withheld.length === 0) {
    return null;
  }
  return (
    <ul className="magic-withheld">
      {withheld.map((missing) => (
        <li className="t-caption" key={`${missing.source}-${missing.reason}`}>
          {sourceUnavailableText(missing, t)}
        </li>
      ))}
    </ul>
  );
}

/**
 * What this read deliberately left out, by kind and count.
 *
 * Said rather than dropped, because a page showing five lines that never
 * mentions the sixth is a completeness claim nobody made.
 */
function NotShown({
  entries,
}: Readonly<{ entries: readonly MagicNotShown[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  return (
    <ul className="magic-notshown">
      {entries.map((entry) => (
        <li className="t-caption" key={entry.reason}>
          {plural("magic.notShown", entry.count, {
            count: formatNumber(entry.count, locale),
            reason: t(NOT_SHOWN_REASON[entry.reason]),
          })}
        </li>
      ))}
    </ul>
  );
}
