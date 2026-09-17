// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useRecordZone } from "../app/recordzone";
import { middayInstant } from "../format/calendarday";
import { formatDate, formatDayLong, hourInZone } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { WeeklyReview } from "./brief.queries";
import {
  type BriefSentence,
  briefSentence,
  sentenceParts,
} from "./brief.sentence";
import type { BriefView } from "./brief.view";
import { weekSentence } from "./brief.weeksentence";
import type { Worklist } from "./worklist.queries";

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
  scope?: "mine" | "team";
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
function GlanceSentence({ sentence }: Readonly<{ sentence: BriefSentence }>) {
  const t = useT();
  const parts = sentenceParts(t(sentence.key));
  return (
    <p className="glance-sentence" data-testid="glance-sentence">
      {/* A template's own words go in as STRINGS rather than wrapped in spans:
          only the filled holes are elements, and a hole's name is its identity
          — `lead`, `consequence`, `rest` appear once each. */}
      {parts.map((part) =>
        part.kind === "text" ? part.text : filled(part.name, sentence, t),
      )}
    </p>
  );
}

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
): ReactNode {
  const value = sentence.values[name];
  if (value === undefined) {
    return `{${name}}`;
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

export function BriefGlance({
  firstName,
  now,
  day,
  week,
  view,
  scope = "mine",
}: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  const recordZone = useRecordZone();
  const { locale } = useLocale();
  const greeting =
    view === "weekly"
      ? t("brief.panel.weekly")
      : firstName
        ? t(greetingKey(hour), { name: firstName })
        : t(anonGreetingKey(hour));
  const sentence = glanceSentence({ view, scope, day, week }, t, locale);
  const date = view === "morning" ? now.toISOString() : week?.local_week_start;
  return (
    <header className="glance arrive" data-testid="brief-glance">
      <h1 className="glance-greeting t-display">{greeting}</h1>
      {date && (
        <p className="t-eyebrow glance-date">
          <time dateTime={date}>
            {/* The morning names its day the way a page over a greeting does
                — the weekday and the month written out — because a reader
                arriving at their desk asks "what day is it" before "what is
                the date". The weekly keeps the numeric form: it names a
                week's start, which is a date a reader compares, not a day
                they are in. */}
            {view === "weekly"
              ? formatDate(middayInstant(date, recordZone), locale, recordZone)
              : formatDayLong(date, locale, viewerZone())}
          </time>
        </p>
      )}
      {sentence ? (
        <GlanceSentence sentence={sentence} />
      ) : (
        // Neither view could compose a sentence: the queue or the week has not
        // arrived, or the read failed. The fallback names the view rather than
        // describing it, because there is nothing yet to describe — and the
        // morning's "this is your day" read as the wrong week entirely beneath
        // a weekly. Each view says its own.
        //
        // ONE FACE either way. It is the same slot the composed sentence fills,
        // and drawn as a caption it read as a footnote where the composed one
        // read as the page's opening.
        <p className="glance-sentence">{t(introKey(view, scope))}</p>
      )}
    </header>
  );
}

function glanceSentence(
  facts: Pick<GlanceFacts, "view" | "scope" | "day" | "week">,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
) {
  if (facts.view === "morning") return briefSentence(facts.day, t, locale);
  return facts.scope === "team" ? null : weekSentence(facts.week, t);
}

function introKey(view: BriefView, scope: "mine" | "team"): MessageKey {
  if (view === "weekly")
    return scope === "team"
      ? "brief.glance.introTeamWeekly"
      : "brief.glance.introWeekly";
  return scope === "team" ? "brief.glance.introTeam" : "brief.glance.intro";
}
