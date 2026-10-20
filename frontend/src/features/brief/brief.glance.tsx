// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { Badge } from "../../design-system/atoms";
import { EmailReference } from "../../design-system/emailreference";
import { Heading } from "../../design-system/heading";
import { RecordChip } from "../../design-system/recordchip";
import { isoWeekOf } from "../../format/calendarday";
import {
  formatDateTime,
  formatDayLong,
  formatDayRange,
  formatNumber,
  formatTimeOfDay,
  hourInZone,
} from "../../format/format";
import { startOfDayInZone, viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import type { Worklist } from "../worklist/worklist.queries";
import type { WeeklyReview } from "./brief.queries";
import {
  type BriefSentence,
  briefSentence,
  meetingSentence,
  sentenceParts,
} from "./brief.sentence";
import type { BriefView } from "./brief.view";
import { weekSentence } from "./brief.weeksentence";

const TODAY_SECTION = "brief-today";

// Four greetings, and the boundaries are the reader's day rather than the
// clock's quarters: work starts before noon, the afternoon runs to the end of
// the working day, the evening to the end of the waking one, and what is left
// over is somebody working at an hour nobody should have to.
function greetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "brief.glance.morning";
  }
  if (hour >= 12 && hour < 18) {
    return "brief.glance.afternoon";
  }
  if (hour >= 18 && hour < 22) {
    return "brief.glance.evening";
  }
  return "brief.glance.night";
}

function anonGreetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "brief.glance.morningAnon";
  }
  if (hour >= 12 && hour < 18) {
    return "brief.glance.afternoonAnon";
  }
  if (hour >= 18 && hour < 22) {
    return "brief.glance.eveningAnon";
  }
  return "brief.glance.nightAnon";
}

/** The greeting for the reader at this hour, by name where one is known. */
export function greetingText(
  firstName: string | null,
  hour: number,
  t: (key: MessageKey, values?: Record<string, string>) => string,
): string {
  return firstName
    ? t(greetingKey(hour), { name: firstName })
    : t(anonGreetingKey(hour));
}

/**
 * The greeting with the reader's name in the accent ink. The name is found in
 * the translated line rather than spliced in, so each language keeps its own
 * word order around it.
 */
export function Greeting({
  text,
  firstName,
}: Readonly<{ text: string; firstName: string | null }>) {
  const at = firstName ? text.lastIndexOf(firstName) : -1;
  if (!firstName || at < 0) return text;
  return (
    <>
      {text.slice(0, at)}
      <span className="greeting-name">{firstName}</span>
      {text.slice(at + firstName.length)}
    </>
  );
}

export type GlanceFacts = Readonly<{
  /** The reader's own name, or null while `/me` is still in flight. */
  firstName: string | null;
  now: Date;
  /** The ranked queue this page is showing, for the opening sentence. Undefined
   *  while it is in flight or after it failed — the sentence is then absent
   *  rather than guessed at. */
  day: Worklist | undefined;
  /** The week that closed, for the weekly's opening sentence. Null when no week
   *  has been written yet; undefined while the read is in flight or after it
   *  failed — a quiet week and an unread one are different sentences. */
  week: WeeklyReview | null | undefined;
  /** Which Brief this is. Each view composes its own sentence: the morning's
   *  from the ranked queue, the weekly's from the counts the week was frozen
   *  with. Neither can describe the other. */
  view: BriefView;
  /** When the agent wrote the morning brief, for the provenance badge. Only
   *  the reader's own morning carries one. */
  briefedAt?: string;
  /** The page's controls, on one line with the date over the greeting. Absent,
   *  the date stands alone over it. */
  controls?: ReactNode;
  /** What stands at the end of the title's line: the weekly's week stepper. */
  titleAside?: ReactNode;
}>;

export type GlanceProps = GlanceFacts;

/**
 * Scroll the day's own order under the reader's eye.
 *
 * A BUTTON rather than an anchor, and the reason is the address bar: every
 * `href` in this product is a route (`#/worklist`), so a fragment link would
 * replace the route with `#brief-today` and send the reader off the page the
 * tail of the sentence is pointing AT.
 */
function showToday(): void {
  // jsdom has no scrollIntoView; the browser always does.
  document.getElementById(TODAY_SECTION)?.scrollIntoView?.({ block: "start" });
}

/**
 * The day in one sentence, with the two clauses a reader can act on made
 * reachable: the lead opens its own record, and the tail reaches today's order.
 *
 * The template is translated with its holes INTACT and cut apart here, because
 * a string with the holes already filled has nowhere to put a link and a
 * sentence assembled from clauses would produce German in English word order.
 */
function GlanceSentence({
  sentence,
  meeting,
}: Readonly<{ sentence: BriefSentence; meeting: BriefSentence | null }>) {
  const t = useT();
  const [opened, setOpened] = useState<Opened>(null);
  const chips: Chips = {
    opened,
    press: (name) => setOpened(opened === name ? null : name),
  };
  const words = (clause: BriefSentence) =>
    sentenceParts(t(clause.key)).map((part) =>
      part.kind === "text" ? part.text : filled(part.name, clause, t, chips),
    );
  const source =
    opened === "who"
      ? sentence.who?.source
      : opened === "meeting"
        ? meeting?.meeting?.source
        : undefined;
  return (
    <>
      <p className="glance-sentence" data-testid="glance-sentence">
        {/* A template's own words go in as STRINGS rather than wrapped in spans:
            only the filled holes are elements, and a hole's name is its
            identity — each appears once. */}
        {words(sentence)}
        {meeting && <> {words(meeting)}</>}
      </p>
      {source && (
        <p className="glance-quote" role="note">
          {source.kind === "email" ? (
            <EmailReference
              subject={source.subject}
              occurredAt={source.occurredAt}
              withheld={source.withheld}
            />
          ) : (
            <>
              <span className="glance-quote-from">{source.from}</span> ·{" "}
              {source.text}
            </>
          )}
        </p>
      )}
    </>
  );
}

/** Which chip in the sentence has its source open. */
type Opened = "who" | "meeting" | null;

type Chips = Readonly<{ opened: Opened; press: (name: Opened) => void }>;

/**
 * What goes in one hole of the sentence.
 *
 * A hole with no value renders its own name back, which is exactly what
 * `translate` does with one: a sentence missing a clause reads as an obvious
 * defect on the page rather than as a sentence that quietly lost a word.
 */
function filled(
  name: string,
  sentence: BriefSentence,
  t: (key: MessageKey, values?: Record<string, string>) => string,
  chips: Chips,
): ReactNode {
  const value = sentence.values[name];
  if (value === undefined) {
    return `{${name}}`;
  }
  const record =
    name === "who"
      ? sentence.who
      : name === "meeting"
        ? sentence.meeting
        : undefined;
  if (record?.source) {
    const opened = name === "who" ? "who" : "meeting";
    return (
      <RecordChip
        key={name}
        kind={name === "who" ? "contact" : "meeting"}
        label={record.label}
        identity={record.id}
        pressed={chips.opened === opened}
        onPress={() => chips.press(opened)}
        size="large"
      />
    );
  }
  if (name === "lead" && sentence.leadHref !== undefined) {
    return (
      <a key={name} className="entity-link" href={sentence.leadHref}>
        {value}
      </a>
    );
  }
  if (name === "rest") {
    // The WHOLE clause is the control, not the numeral inside it: "4" alone is
    // a two-character press target and reads as a figure rather than as a way
    // anywhere.
    //
    // `glance-rest` is what puts it back INTO the sentence: `.link-button` is
    // an inline-flex box with side padding, which is right for a verb standing
    // on its own and wrong for a clause mid-sentence — it printed a space
    // before the full stop that follows it, and its box raised the line the
    // clamp then reserved two of.
    return (
      <button
        key={name}
        type="button"
        className="link-button glance-rest"
        onClick={showToday}
      >
        {t("brief.sentence.rest", { count: value })}
      </button>
    );
  }
  return value;
}

/**
 * The line over the title. The morning names its day written out; the weekly
 * names the report, its calendar week, and when it was frozen.
 */
function GlanceKicker({
  view,
  date,
  week,
}: Readonly<{ view: BriefView; date: string; week: GlanceFacts["week"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <p className="t-eyebrow glance-date">
      <time dateTime={date}>
        {view === "weekly"
          ? t("brief.report.kicker", {
              week: formatNumber(isoWeekOf(date), locale),
            })
          : formatDayLong(date, locale, viewerZone())}
      </time>
      {/* Frozen: written when the week closed, and it can no longer move.
          Only over a review that exists. */}
      {view === "weekly" && week?.generated_at && (
        <>
          {" · "}
          <span className="glance-frozen">
            {t("brief.report.frozenAt", {
              at: formatDateTime(week.generated_at, locale, recordZone),
            })}
          </span>
        </>
      )}
    </p>
  );
}

export function BriefGlance({
  firstName,
  now,
  day,
  week,
  view,
  briefedAt,
  controls,
  titleAside,
}: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  const recordZone = useRecordZone();
  const { locale } = useLocale();
  const weekStart = view === "weekly" ? week?.local_week_start : undefined;
  // The weekly is a report: its title is the week it covers, and before any
  // week has closed it names the report itself.
  const greeting =
    view === "weekly"
      ? weekStart
        ? formatDayRange(
            weekStart,
            startOfDayInZone(weekStart, recordZone, 6),
            locale,
            recordZone,
          )
        : t("brief.panel.weekly")
      : greetingText(firstName, hour, t);
  const sentence = glanceSentence({ view, day, week }, t, locale);
  const meeting =
    view === "morning" ? meetingSentence(day, now, t, locale) : null;
  const date = view === "morning" ? now.toISOString() : week?.local_week_start;
  return (
    <header className="glance arrive" data-testid="brief-glance">
      <div className="glance-title">
        <Heading size="xlarge" className="glance-greeting t-display">
          {view === "weekly" ? (
            greeting
          ) : (
            <Greeting text={greeting} firstName={firstName} />
          )}
        </Heading>
        {titleAside}
      </div>
      {(date || controls) && (
        <div className="glance-topline">
          {date && <GlanceKicker view={view} date={date} week={week} />}
          {controls}
        </div>
      )}
      {!narratedWeek(view, week) && (
        <GlanceOpening view={view} sentence={sentence} meeting={meeting} />
      )}
      {briefedAt && (
        <p className="glance-briefed">
          <Badge tone="ai">
            {t("brief.briefedAt", {
              time: formatTimeOfDay(briefedAt, locale, viewerZone()),
            })}
          </Badge>
        </p>
      )}
    </header>
  );
}

/** A week Margince narrated opens on that narration, under the head; the
 *  counted sentence stands in only where nobody wrote one. */
function narratedWeek(view: BriefView, week: GlanceFacts["week"]): boolean {
  return view === "weekly" && Boolean(week?.narrated_at && week.narrative);
}

function GlanceOpening({
  view,
  sentence,
  meeting,
}: Readonly<{
  view: BriefView;
  sentence: BriefSentence | null;
  meeting: BriefSentence | null;
}>) {
  const t = useT();
  if (sentence) return <GlanceSentence sentence={sentence} meeting={meeting} />;
  // No sentence yet: the read is in flight or failed. The fallback names the
  // view, since there is nothing to describe, in the same face as a sentence.
  return <p className="glance-sentence">{t(introKey(view))}</p>;
}

function glanceSentence(
  facts: Pick<GlanceFacts, "view" | "day" | "week">,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
) {
  if (facts.view === "morning") return briefSentence(facts.day, t, locale);
  return weekSentence(facts.week, t, locale);
}

function introKey(view: BriefView): MessageKey {
  return view === "weekly" ? "brief.glance.introWeekly" : "brief.glance.intro";
}
