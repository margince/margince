// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight } from "lucide-react";
import type { ReactNode } from "react";
import { Eyebrow } from "../design-system/eyebrow";
import { type CappedCount, cappedCountLabel } from "../format/cappedcount";
import { hourInZone } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { briefSentence } from "./brief.sentence";
import type { BriefView } from "./brief.view";
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

/** What is waiting for a decision, and how much of it stops waiting today. */
export type GlanceDecisions = {
  pending: number;
  expiringToday: number;
};

/** The ranked queue, and the deal at the top of it. */
export type GlanceBrief = {
  ranked: number;
  /** Both halves or neither: a named deal with no figure reads as unpriced. */
  topDeal: string | null;
  topAmount: string | null;
};

/** What the night shift did while the reader was away. */
export type GlanceOvernight = {
  captured: number;
  duplicates: number;
};

export type GlanceFacts = Readonly<{
  /** The reader's own name, or null while `/me` is still in flight. */
  firstName: string | null;
  now: Date;
  /** Null when the reading could not be taken — never zero-as-unknown. */
  decisions: GlanceDecisions | null;
  brief: GlanceBrief | null;
  overnight: GlanceOvernight | null;
  /** Open deals that have gone quiet. */
  /** Open deals gone quiet, as a floor: Home reads one page of deals. */
  stalled: CappedCount | null;
  /** The ranked queue this page is showing, for the opening sentence. Undefined
   *  while it is in flight or after it failed — the sentence is then absent
   *  rather than guessed at. */
  day: Worklist | undefined;
  /** Which Brief this is. The eyebrow names it, and the sentence belongs to the
   *  morning: it is composed from the ranked queue, which says nothing about a
   *  week that has closed. */
  view: BriefView;
}>;

export type GlanceProps = GlanceFacts &
  Readonly<{
    /** Jump to the decisions deck on this page. */
    onGoToDecisions: () => void;
    /** Jump to the ranked queue on this page. */
    onGoToToday: () => void;
    /** Leave for the duplicates queue, which is its own screen. */
    onGoToDuplicates: () => void;
    /** Jump to the quiet-deals panel on this page. */
    onGoToWatch: () => void;
  }>;

/**
 * A figure inside a briefing line, and the way to the thing it counts.
 *
 * A button rather than a styled span: every one of these is somewhere the
 * reader can go, and a number that looks pressable and is not is worse than a
 * plain number. Kept local to Home deliberately — one caller, and this repo
 * does not promote a primitive into the design system before a second surface
 * needs it.
 */
function CountMark({
  count,
  more,
  onClick,
  label,
}: Readonly<{
  count: number;
  /** Whether the figure is a floor rather than a total — the reading came off
   *  one page and there was another behind it. */
  more?: boolean;
  onClick?: () => void;
  label?: string;
}>) {
  const { locale } = useLocale();
  // A figure with nowhere to go is a figure, not a control. The overnight
  // capture count is the case: Home has no destination that lists the messages
  // it counts, and pointing it at the dedupe queue sent a reader to a different
  // dataset from the number they pressed. It keeps the numeral's ground and
  // loses the arrow and the press target, which is the honest reading.
  if (!onClick) {
    return (
      <span className="glance-count glance-count-flat t-mono">
        {cappedCountLabel({ seen: count, more: more ?? false }, locale)}
      </span>
    );
  }
  // The destination goes in a spoken span rather than an `aria-label`: a label
  // REPLACES the content, so the figure — the one thing the reader pressed —
  // would be the part assistive technology never says, and two counts pointing
  // at two places would announce the same name.
  return (
    <button type="button" className="glance-count t-mono" onClick={onClick}>
      {cappedCountLabel({ seen: count, more: more ?? false }, locale)}
      {label ? <span className="sr-only"> {label}</span> : null}
      <ArrowRight size={13} aria-hidden="true" />
    </button>
  );
}

/** One line of the briefing: a numeral that goes somewhere, then the clause. */
function GlanceLine({
  count,
  more,
  onClick,
  goLabel,
  children,
  testId,
}: Readonly<{
  count: number;
  /** Whether the figure is a floor — forwarded to `CountMark`. */
  more?: boolean;
  /** Omitted where the figure has no destination — see `CountMark`. */
  onClick?: () => void;
  goLabel?: string;
  children: ReactNode;
  testId: string;
}>) {
  return (
    <p className="glance-line" data-testid={testId}>
      <CountMark count={count} more={more} onClick={onClick} label={goLabel} />
      <span className="glance-clause">{children}</span>
    </p>
  );
}

export function HomeGlance({
  firstName,
  now,
  day,
  view,
  decisions,
  brief,
  overnight,
  stalled,
  onGoToDecisions,
  onGoToToday,
  onGoToDuplicates,
  onGoToWatch,
}: GlanceProps) {
  const plural = usePlural();
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
  // MORNING ONLY. It is composed from the ranked queue, which is what waits
  // TODAY — over the weekly it would be describing this morning under a heading
  // about the week that closed.
  const sentence = view === "morning" ? briefSentence(day, t, locale) : null;

  return (
    <header className="glance arrive" data-testid="home-glance">
      {/* Scope and date, above the greeting. A span rather than a heading: the
          page has ONE h1 and this is its label, not a level of its own. */}
      <Eyebrow className="glance-eyebrow">
        {t(view === "weekly" ? "brief.eyebrow.weekly" : "brief.eyebrow")}
      </Eyebrow>
      <h1 className="glance-greeting t-display">{greeting}</h1>
      {sentence ? (
        <p className="glance-sentence" data-testid="glance-sentence">
          {t(sentence.key, sentence.values)}
        </p>
      ) : (
        <p className="glance-intro t-caption">{t("home.glance.intro")}</p>
      )}
      <div className="glance-lines">
        {decisions !== null && decisions.pending === 0 && (
          <p className="glance-line glance-line-plain">
            {t("home.glance.decisionsClear")}
          </p>
        )}
        {decisions !== null && decisions.pending > 0 && (
          <GlanceLine
            testId="glance-decisions"
            count={decisions.pending}
            onClick={onGoToDecisions}
            goLabel={t("home.glance.goDecisions")}
          >
            {plural("home.glance.decisions", decisions.pending)}
          </GlanceLine>
        )}
        {decisions !== null && decisions.expiringToday > 0 && (
          <GlanceLine
            testId="glance-expiring"
            count={decisions.expiringToday}
            onClick={onGoToDecisions}
            goLabel={t("home.glance.goDecisions")}
          >
            {plural("home.glance.expiring", decisions.expiringToday)}
          </GlanceLine>
        )}
        {brief !== null && brief.ranked > 0 && (
          <GlanceLine
            testId="glance-ranked"
            count={brief.ranked}
            onClick={onGoToToday}
            goLabel={t("home.glance.goToday")}
          >
            {plural("home.glance.ranked", brief.ranked)}{" "}
            {/* The leader is named only when both halves are known. A deal
                named without its figure reads as one nobody priced, and a
                figure without the name belongs to no deal at all. */}
            {brief.topDeal && brief.topAmount && (
              <span className="glance-leader">
                {t("home.glance.leader", {
                  deal: brief.topDeal,
                  amount: brief.topAmount,
                })}
              </span>
            )}
          </GlanceLine>
        )}
        {overnight !== null && (
          <GlanceLine testId="glance-captured" count={overnight.captured}>
            {plural("home.glance.captured", overnight.captured)}
          </GlanceLine>
        )}
        {overnight !== null && overnight.duplicates > 0 && (
          <GlanceLine
            testId="glance-duplicates"
            count={overnight.duplicates}
            onClick={onGoToDuplicates}
            goLabel={t("home.glance.goDuplicates")}
          >
            {plural("home.glance.duplicates", overnight.duplicates)}
          </GlanceLine>
        )}
        {stalled !== null && stalled.seen > 0 && (
          <GlanceLine
            testId="glance-quiet"
            count={stalled.seen}
            more={stalled.more}
            onClick={onGoToWatch}
            goLabel={t("home.glance.goWatch")}
          >
            {plural("home.glance.quiet", stalled.seen)}
          </GlanceLine>
        )}
      </div>
    </header>
  );
}
