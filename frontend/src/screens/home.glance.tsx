// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight } from "lucide-react";
import type { ReactNode } from "react";
import { hourInZone } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

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
  stalled: number | null;
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
  onClick,
  label,
}: Readonly<{ count: number; onClick?: () => void; label?: string }>) {
  // A figure with nowhere to go is a figure, not a control. The overnight
  // capture count is the case: Home has no destination that lists the messages
  // it counts, and pointing it at the dedupe queue sent a reader to a different
  // dataset from the number they pressed. It keeps the numeral's ground and
  // loses the arrow and the press target, which is the honest reading.
  if (!onClick) {
    return (
      <span className="glance-count glance-count-flat t-mono">{count}</span>
    );
  }
  return (
    <button
      type="button"
      className="glance-count t-mono"
      onClick={onClick}
      aria-label={label}
    >
      {count}
      <ArrowRight size={13} aria-hidden="true" />
    </button>
  );
}

/** One line of the briefing: a numeral that goes somewhere, then the clause. */
function GlanceLine({
  count,
  onClick,
  goLabel,
  children,
  testId,
}: Readonly<{
  count: number;
  /** Omitted where the figure has no destination — see `CountMark`. */
  onClick?: () => void;
  goLabel?: string;
  children: ReactNode;
  testId: string;
}>) {
  return (
    <p className="glance-line" data-testid={testId}>
      <CountMark count={count} onClick={onClick} label={goLabel} />
      <span className="glance-clause">{children}</span>
    </p>
  );
}

export function HomeGlance({
  firstName,
  now,
  decisions,
  brief,
  overnight,
  stalled,
  onGoToDecisions,
  onGoToToday,
  onGoToDuplicates,
  onGoToWatch,
}: GlanceProps) {
  const t = useT();
  const hour = hourInZone(now, viewerZone());
  // No name yet is not a reason to greet nobody: the hour is known either way,
  // and the name arrives a moment later without the heading having to move.
  const greeting = firstName
    ? t(greetingKey(hour), { name: firstName })
    : t(anonGreetingKey(hour));

  return (
    <header className="glance arrive" data-testid="home-glance">
      <h1 className="glance-greeting t-display">{greeting}</h1>
      <p className="glance-intro t-caption">{t("home.glance.intro")}</p>
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
            {t(
              decisions.pending === 1
                ? "home.glance.decisions.one"
                : "home.glance.decisions.other",
            )}
          </GlanceLine>
        )}
        {decisions !== null && decisions.expiringToday > 0 && (
          <GlanceLine
            testId="glance-expiring"
            count={decisions.expiringToday}
            onClick={onGoToDecisions}
            goLabel={t("home.glance.goDecisions")}
          >
            {t(
              decisions.expiringToday === 1
                ? "home.glance.expiring.one"
                : "home.glance.expiring.other",
            )}
          </GlanceLine>
        )}
        {brief !== null && brief.ranked > 0 && (
          <GlanceLine
            testId="glance-ranked"
            count={brief.ranked}
            onClick={onGoToToday}
            goLabel={t("home.glance.goToday")}
          >
            {t(
              brief.ranked === 1
                ? "home.glance.ranked.one"
                : "home.glance.ranked.other",
            )}{" "}
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
            {t(
              overnight.captured === 1
                ? "home.glance.captured.one"
                : "home.glance.captured.other",
            )}
          </GlanceLine>
        )}
        {overnight !== null && overnight.duplicates > 0 && (
          <GlanceLine
            testId="glance-duplicates"
            count={overnight.duplicates}
            onClick={onGoToDuplicates}
            goLabel={t("home.glance.goDuplicates")}
          >
            {t(
              overnight.duplicates === 1
                ? "home.glance.duplicates.one"
                : "home.glance.duplicates.other",
            )}
          </GlanceLine>
        )}
        {stalled !== null && stalled > 0 && (
          <GlanceLine
            testId="glance-quiet"
            count={stalled}
            onClick={onGoToWatch}
            goLabel={t("home.glance.goWatch")}
          >
            {t(
              stalled === 1
                ? "home.glance.quiet.one"
                : "home.glance.quiet.other",
            )}
          </GlanceLine>
        )}
      </div>
    </header>
  );
}
