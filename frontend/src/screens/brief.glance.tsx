// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { hourInZone } from "../format/format";
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

// The first thing a reader sees each morning: who they are, and the day stated
// in one sentence. The readings strip directly below says the numbers; this
// block exists to say what to DO about the first of them, which a row of
// figures cannot.
//
// TWO LINES AND NOTHING ELSE. What stood here before — an uppercase eyebrow
// naming the view, a clock reporting the minute the queue was read, and the
// date under the greeting — were three lines a reader already knew. The view is
// the dial's own state, drawn beside this block; the date is the shell's; and
// an as-of that ticked every sixty seconds re-rendered the page's opening for a
// digit nobody was reading. Both views are now greeting plus sentence, which is
// also why neither wears a label: two views drawn alike need no kicker to tell
// them apart, and one of them wearing one would be the odd page.
//
// Presentational and total: every figure arrives as a prop and every prop is
// nullable, because "we could not read this" and "there is none of it" are
// different sentences and neither may be printed as a zero.
//
// `now` is a prop rather than a call to the clock inside the render. The
// greeting is the one thing here that changes with the hour, so a test that
// cannot choose the hour cannot test it, and a real clock would make the same
// test pass at 09:00 and fail at 21:00.

/** Where the day's own order is drawn, for the sentence's tail to reach. */
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

export function BriefGlance({ firstName, now, day, week, view }: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  // No name yet is not a reason to greet nobody: the hour is known either way,
  // and the name arrives a moment later without the heading having to move.
  const greeting = firstName
    ? t(greetingKey(hour), { name: firstName })
    : t(anonGreetingKey(hour));

  // EACH VIEW COMPOSES ITS OWN. The morning's comes from the ranked queue,
  // which is what waits TODAY; over the weekly it would be describing this
  // morning under a heading about the week that closed. The weekly's comes from
  // the frozen counts, which is what the week is now a record of.
  const { locale } = useLocale();
  const sentence =
    view === "morning" ? briefSentence(day, t, locale) : weekSentence(week, t);

  return (
    <header className="glance arrive" data-testid="brief-glance">
      <h1 className="glance-greeting t-display">{greeting}</h1>
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
        <p className="glance-sentence">
          {t(
            view === "weekly"
              ? "brief.glance.introWeekly"
              : "brief.glance.intro",
          )}
        </p>
      )}
    </header>
  );
}
