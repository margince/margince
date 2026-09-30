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
// A PLAIN PANEL, and Home's only receipt: the page hands in the changes that
// wait for a word and the night's digest, so "what happened while I was away"
// is answered in one place rather than three.

import type { LucideIcon } from "lucide-react";
import { CircleAlert, CircleCheck, CircleDashed, Sparkles } from "lucide-react";
import { type ReactNode, useState } from "react";
import { ENTITY, isEntityKind } from "../app/entity";
import { routeHash } from "../app/router";
import { SegmentedControl } from "../design-system/atoms";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type PluralBase, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  magicByKey,
  magicConsequenceKey,
  magicSentenceKey,
  magicWhyKey,
} from "./magic.keys";
import {
  MAGIC_WINDOWS,
  type MagicLane,
  type MagicLine,
  type MagicNotShown,
  type MagicReceipt,
  type MagicWindow,
  useMagic,
} from "./magic.queries";
import { LineRecordsOpener } from "./magic.records";
import { MagicUndoButton } from "./magic.undo";
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
  const drawn = LANES.filter((lane) => hasLines(receipt?.[lane]));
  const state = magic.isPending
    ? "loading"
    : magic.isError
      ? "failed"
      : "ready";
  return (
    <Panel
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
        {receipt?.since && (
          <PanelIntro>
            {t("magic.intro", {
              when: formatDateTime(receipt.since, locale, zone),
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
        </SurfaceState>
      </PanelBody>
      {lead}
      {drawn.map((lane) => (
        <MagicLaneSection
          key={lane}
          lane={lane}
          rows={receipt?.[lane] ?? []}
          since={receipt?.since}
          zone={zone}
        />
      ))}
      {(receipt?.not_shown?.length ?? 0) > 0 && (
        <PanelBody>
          <NotShown entries={receipt?.not_shown ?? []} />
        </PanelBody>
      )}
      {foot}
    </Panel>
  );
}

// A lane off a payload this client cannot read is not an empty one: absent is
// version skew, and only a list the server sent can say there is nothing in it.
function hasLines(rows: readonly MagicLine[] | undefined): boolean {
  return Array.isArray(rows) && rows.length > 0;
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
    // THIS PAGE's count, which is all the endpoint promises, and the rows
    // themselves when a server older or newer than this client sends none.
    const count = receipt.totals?.[lane] ?? rows.length;
    return {
      lane,
      tone: LANE_TONE[lane],
      text: plural(LANE_COUNT[lane], count, {
        count: formatNumber(count, locale),
      }),
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

function MagicLaneSection({
  lane,
  rows,
  since,
  zone,
}: Readonly<{
  lane: MagicLane;
  rows: readonly MagicLine[];
  since: string | undefined;
  zone: string;
}>) {
  const t = useT();
  return (
    <>
      <PanelGroupHead title={t(LANE_HEADING[lane])} level="h3" />
      <PanelBody>
        <ul className="magic-lines" aria-label={t(LANE_HEADING[lane])}>
          {rows.map((row) => (
            // The lanes mint their ids independently, so an id alone can name a
            // row in a lane the reader was not looking at.
            <li className="magic-line" key={`${lane}-${row.id}`}>
              <div className="magic-line-text">
                <LineSentence line={row} />
                <LineMeta line={row} since={since} zone={zone} />
              </div>
              <div className="magic-line-back">
                <LineWayBack line={row} />
              </div>
            </li>
          ))}
        </ul>
      </PanelBody>
    </>
  );
}

/**
 * The record the line is about, who acted, and when: each its own element, so
 * the separators between them are the stylesheet's and never part of a value.
 */
function LineMeta({
  line,
  since,
  zone,
}: Readonly<{ line: MagicLine; since: string | undefined; zone: string }>) {
  const t = useT();
  const by = line.actor.label ? magicByKey(line.actor.label.key) : null;
  return (
    <p className="t-caption magic-line-meta">
      <span>
        <LineSubject line={line} since={since} />
      </span>
      {by && <span>{t(by, line.actor.label?.values)}</span>}
      <span>
        <LineWhen line={line} zone={zone} />
      </span>
    </p>
  );
}

/**
 * What happened, and what it means for the reader.
 *
 * A sentence this build has no key for is DROPPED rather than printed: a newer
 * server mints keys this client predates, and `magic.action.something` on a
 * receipt is worse than a row that says only what it was about and when.
 */
function LineSentence({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  const sentence = magicSentenceKey(line.summary.key);
  const consequence = line.consequence
    ? magicConsequenceKey(line.consequence)
    : null;
  // WHY the machinery did it, under what it did. A reason key this build
  // predates draws nothing, for the reason an unknown sentence does.
  const why = line.reason ? magicWhyKey(line.reason.key) : null;
  return (
    <>
      {sentence && <span>{t(sentence, line.summary.values)}</span>}
      {why && line.reason && (
        <p className="t-caption">{t(why, line.reason.values)}</p>
      )}
      {consequence && <p className="t-caption">{t(consequence)}</p>}
    </>
  );
}

/**
 * When it happened, and for a watched source how long it has been that way.
 *
 * A watching line's occurred_at is when the condition was OBSERVED, so printing
 * it alone would date every outage to this page load. Where the condition has a
 * beginning the server sends it, and that is the figure a reader acts on.
 */
function LineWhen({ line, zone }: Readonly<{ line: MagicLine; zone: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const began = line.summary.values?.failing_since;
  if (line.lane === "watching" && began) {
    return t("magic.failingSince", {
      when: formatDateTime(began, locale, zone),
    });
  }
  return formatDateTime(line.occurred_at, locale, zone);
}

/**
 * Which record the line is about, as a link where the app has a page for it.
 *
 * The subject is its own value rather than a word inside the sentence:
 * interpolating a record's name into a translated clause is how a sentence
 * ends up ungrammatical in two of three languages.
 */
function LineSubject({
  line,
  since,
}: Readonly<{ line: MagicLine; since: string | undefined }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const label = subjectLabel(line);
  const count = line.count ?? 1;
  // A done line about records OPENS: to every record it stands for, what
  // changed on each, and an undo per record. Retention names no record, and a
  // line from another lane is not a change to inspect.
  const opens = line.lane === "done" && line.entity !== undefined && since;
  // ONE line for a job that touched many records: the most recent one by
  // name, and how many more.
  if (count > 1 || (!line.entity && line.count !== undefined)) {
    const summary =
      label && count > 1
        ? plural("magic.aboutMany", count - 1, {
            label,
            others: formatNumber(count - 1, locale),
          })
        : plural("magic.aboutCount", count, {
            count: formatNumber(count, locale),
          });
    return opens ? (
      <LineRecordsOpener line={line} since={since} summary={summary} />
    ) : (
      summary
    );
  }
  if (!label) {
    return t("magic.noRecord");
  }
  const href = recordHref(line);
  return (
    <>
      {href ? <a href={href}>{label}</a> : label}
      {opens && (
        <LineRecordsOpener
          line={line}
          since={since}
          summary={t("magic.records.show")}
        />
      )}
    </>
  );
}

/**
 * The way back: an undo for a change to one record, done here with one press.
 * A line standing for many records offers its undos inside, one per record —
 * one button that put back 150 changes nobody had looked at would be the same
 * unasked bulk write this page exists to report.
 */
function LineWayBack({ line }: Readonly<{ line: MagicLine }>) {
  const t = useT();
  if ((line.count ?? 1) > 1 && line.entity) {
    return <span className="t-caption">{t("magic.undo.perRecord")}</span>;
  }
  // A line naming no record still says why it cannot be taken back; it offers
  // no press, because the restore route needs a record to name.
  return (
    <MagicUndoButton
      undo={line.undo}
      entityType={line.entity?.type ?? ""}
      entityId={line.entity?.id ?? ""}
    />
  );
}

/**
 * What to call the record this line is about.
 *
 * The entity's own label first, then the caption the approval was staged under,
 * then the mailbox a connector is failing on. Absent rather than invented: a
 * line naming a record it cannot label still says what happened.
 */
function subjectLabel(line: MagicLine): string | undefined {
  const values = line.summary.values;
  return line.entity?.label ?? values?.target ?? values?.account;
}

// The record's address, through the entity registry rather than a switch
// written here: the record types have route names of their own, and a second
// spelling sends a reader to a page that does not exist. An activity resolves
// to nothing on purpose — it is a timeline entry rather than a record with a
// page.
function recordHref(line: MagicLine): string | undefined {
  const entity = line.entity;
  if (!entity || !isEntityKind(entity.type)) {
    return undefined;
  }
  return routeHash(ENTITY[entity.type].route(entity.id));
}
