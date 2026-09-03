// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Eyebrow } from "../design-system/eyebrow";
import { formatTimeOfDay, hourInZone } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { briefSentence } from "./brief.sentence";
import type { BriefView } from "./brief.view";
import { weekSentence } from "./brief.weeksentence";
import type { WeeklyReview } from "./home.queries";
import type { Worklist } from "./worklist.queries";

// The first thing a reader sees each morning: who they are, what hour it is for
// them, and the day stated in sentences. The readings strip directly below says
// the same numbers; this block exists to say what they MEAN, which a row of
// figures cannot.
//
// Presentational and total: every figure arrives as a prop and every prop is
// nullable, because "we could not read this" and "there is none of it" are
// different sentences and neither may be printed as a zero. A reading that is
// missing contributes NO line at all — an absent line is honest, an invented
// zero is not.
//
// `now` is a prop rather than a call to the clock inside the render. The
// greeting is the one thing on Home that changes with the hour, so a test that
// cannot choose the hour cannot test it, and a real clock would make the same
// test pass at 09:00 and fail at 21:00.
//
// Each line reads NUMERAL then sentence, and the numeral is the control. That
// order is also why the sentences are stored per-count (`.one`/`.other`) rather
// than assembled from words: a translator gets a whole clause to agree with the
// numeral in front of it, which is the difference between a sentence and a
// concatenation.

// Four greetings, and the boundaries are the reader's day rather than the
// clock's quarters: work starts before noon, the afternoon runs to the end of
// the working day, the evening to the end of the waking one, and what is left
// over is somebody working at an hour nobody should have to.
function greetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "home.glance.morning";
  }
  if (hour >= 12 && hour < 18) {
    return "home.glance.afternoon";
  }
  if (hour >= 18 && hour < 22) {
    return "home.glance.evening";
  }
  return "home.glance.night";
}

function anonGreetingKey(hour: number): MessageKey {
  if (hour >= 5 && hour < 12) {
    return "home.glance.morningAnon";
  }
  if (hour >= 12 && hour < 18) {
    return "home.glance.afternoonAnon";
  }
  if (hour >= 18 && hour < 22) {
    return "home.glance.eveningAnon";
  }
  return "home.glance.nightAnon";
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
  /** Which Brief this is. The eyebrow names it, and each view composes its own
   *  sentence: the morning's from the ranked queue, the weekly's from the
   *  counts the week was frozen with. Neither can describe the other. */
  view: BriefView;
}>;

export type GlanceProps = GlanceFacts;

// The eyebrow names the view and, for the morning, the moment the queue below
// was read. The as-of is the morning's alone: the weekly's numbers were frozen
// when the week closed, and a time-of-day against them would date the reading
// rather than the week. A morning whose queue has not arrived yet says the
// scope by itself instead of naming a moment it does not know.
function eyebrowText(
  view: BriefView,
  day: Worklist | undefined,
  t: Translator,
  locale: Locale,
): string {
  if (view === "weekly") {
    return t("brief.eyebrow.weekly");
  }
  const scope = t("brief.eyebrow");
  if (day === undefined) {
    return scope;
  }
  return t("brief.eyebrow.asOf", {
    scope,
    at: formatTimeOfDay(day.as_of, locale, viewerZone()),
  });
}

export function HomeGlance({ firstName, now, day, week, view }: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  // No name yet is not a reason to greet nobody: the hour is known either way,
  // and the name arrives a moment later without the heading having to move.
  const greeting = firstName
    ? t(greetingKey(hour), { name: firstName })
    : t(anonGreetingKey(hour));

  const { locale } = useLocale();
  // THE DAY IN ONE SENTENCE, from the rows the page is already showing. The
  // lines below say the same facts as separate counts; this says what to do
  // about the first one, which a column of figures cannot.
  // EACH VIEW COMPOSES ITS OWN. The morning's comes from the ranked queue,
  // which is what waits TODAY; over the weekly it would be describing this
  // morning under a heading about the week that closed. The weekly's comes from
  // the frozen counts, which is what the week is now a record of.
  const sentence =
    view === "morning" ? briefSentence(day, t, locale) : weekSentence(week, t);

  return (
    <header className="glance arrive" data-testid="home-glance">
      {/* Scope and date, above the greeting. A span rather than a heading: the
          page has ONE h1 and this is its label, not a level of its own. */}
      <Eyebrow className="glance-eyebrow">
        {eyebrowText(view, day, t, locale)}
      </Eyebrow>
      <h1 className="glance-greeting t-display">{greeting}</h1>
      {sentence ? (
        <p className="glance-sentence" data-testid="glance-sentence">
          {t(sentence.key, sentence.values)}
        </p>
      ) : (
        // Neither view could compose a sentence: the queue or the week has not
        // arrived, or the read failed. The fallback names the view rather than
        // describing it, because there is nothing yet to describe — and the
        // morning's "this is your day" read as the wrong week entirely beneath
        // "YOUR WEEK". Each view says its own.
        <p className="glance-intro t-caption">
          {t(
            view === "weekly" ? "home.glance.introWeekly" : "home.glance.intro",
          )}
        </p>
      )}
    </header>
  );
}
