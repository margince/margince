// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Fragment, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { MarginceCoreScene } from "../../design-system/margince-core";
import { usePrefersReducedMotion } from "../../design-system/motion";
import {
  formatDayLong,
  formatNumber,
  formatTimeOfDay,
  hourInZone,
} from "../../format/format";
import { dayInZone, viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";
import type { Worklist } from "../worklist/worklist.queries";
import { calendarDay } from "./brief.facts";
import { greetingText } from "./brief.glance";
import { useMorningDigest } from "./brief.queries";
import { meetingsReading } from "./brief.readings.meetings";

import "./brief.intro.css";

/** How long the greeting stands before it lifts away. */
export const SHOW_MS = 4500;
/** How long it takes to lift away and draw up off the page. */
export const LEAVE_MS = 900;
/** How long the day's figures stand when the read lands late in the greeting. */
export const FIGURES_STAND_MS = 2000;
// Where the choreography places the night's line and the first figure, from
// the moment the curtain rises (brief.intro.css).
const READ_AT_MS = 950;
const FIGURES_AT_MS = 1400;
const FIGURE_STEP_MS = 150;

/**
 * The morning's greeting, over the whole page, once a local day.
 *
 * It decides on the first render, from the stored day and the motion setting,
 * so it rises with the page. Its figures join when today's read lands. Any key
 * or press ends it; the page under it is drawn all along.
 */
export function BriefIntro({
  firstName,
  day,
  now,
}: Readonly<{
  firstName: string | null;
  day: Worklist | undefined;
  now: Date;
}>) {
  const reduced = usePrefersReducedMotion();
  const zone = viewerZone();
  const today = dayInZone(now.getTime(), zone);
  const [due] = useState(
    () => readStored(STORAGE_KEYS.briefIntroShown) !== today,
  );
  const [startedAt] = useState(() => Date.now());
  const [ended, setEnded] = useState(false);
  const [leaving, setLeaving] = useState(false);
  // A read from an earlier day would greet with yesterday's figures, so the
  // curtain shows figures only from today's read.
  const fresh =
    day !== undefined && dayInZone(Date.parse(day.as_of), zone) === today;
  const showing = due && !reduced && !ended;

  useEffect(() => {
    if (!showing) return;
    writeStored(STORAGE_KEYS.briefIntroShown, today);
    // A key is a reader who wants the page now, so it ends the greeting at once.
    const end = () => setEnded(true);
    globalThis.addEventListener("keydown", end);
    return () => globalThis.removeEventListener("keydown", end);
  }, [showing, today]);

  // Left alone it lifts away after SHOW_MS. Figures that land late still stand
  // long enough to be read; a read that never lands does not hold it up.
  useEffect(() => {
    if (!showing || leaving) return;
    const elapsed = Date.now() - startedAt;
    const hold = fresh
      ? Math.max(SHOW_MS - elapsed, FIGURES_STAND_MS)
      : SHOW_MS - elapsed;
    const lift = setTimeout(() => setLeaving(true), hold);
    return () => clearTimeout(lift);
  }, [showing, leaving, fresh, startedAt]);

  useEffect(() => {
    if (!leaving) return;
    const timer = setTimeout(() => setEnded(true), LEAVE_MS);
    return () => clearTimeout(timer);
  }, [leaving]);

  if (!showing) return null;
  // On the body, over every layer the shell stacks, the phone's tab bar too.
  return createPortal(
    <IntroCurtain
      firstName={firstName}
      day={fresh ? day : undefined}
      now={now}
      zone={zone}
      startedAt={startedAt}
      leaving={leaving}
      onEnd={() => setEnded(true)}
    />,
    document.body,
  );
}

function IntroCurtain({
  firstName,
  day,
  now,
  zone,
  startedAt,
  leaving,
  onEnd,
}: Readonly<{
  firstName: string | null;
  day: Worklist | undefined;
  now: Date;
  zone: string;
  startedAt: number;
  leaving: boolean;
  onEnd: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const digest = useMorningDigest();
  const emails = digest.data?.capture.messages_synced;
  const greeting = greetingText(firstName, hourInZone(now, zone), t);
  return (
    // Named by its own text, so a screen reader hears the greeting and its
    // figures and not only the way out.
    <button
      type="button"
      className="brief-intro"
      data-state={leaving ? "leaving" : undefined}
      onClick={onEnd}
    >
      <span className="brief-intro-body">
        <span className="brief-intro-topline">
          {/* The agent's orb: it wrote the brief this greeting opens. */}
          <MarginceCoreScene size="md" className="brief-intro-core" />
          <span className="t-eyebrow brief-intro-date">
            {formatDayLong(now.toISOString(), locale, zone)} ·{" "}
            {formatTimeOfDay(now.toISOString(), locale, zone)}
          </span>
        </span>
        <span className="brief-intro-greeting">
          {greetingWords(greeting).map(({ word, key }, index) => (
            // The words arrive one after the next; the spaces between them
            // stay text, so the line still reads as one sentence.
            <Fragment key={key}>
              {index > 0 && " "}
              <span
                className={
                  firstName && word.includes(firstName)
                    ? "brief-intro-word greeting-name"
                    : "brief-intro-word"
                }
                style={{
                  animationDelay: `calc(var(--dur-enter) + ${index} * var(--dur-state))`,
                }}
              >
                {word}
              </span>
            </Fragment>
          ))}
        </span>
        {typeof emails === "number" && emails > 0 && (
          <IntroRead emails={emails} startedAt={startedAt} />
        )}
        <span className="brief-intro-rule" aria-hidden="true" />
        {day ? (
          <IntroFigures day={day} startedAt={startedAt} />
        ) : (
          // Holds the figures' row while the read is on its way, so the
          // greeting does not jump when they land.
          <span className="brief-intro-figures" data-pending aria-hidden="true">
            <span className="brief-intro-figure">
              <span className="brief-intro-value t-num">0</span>
              <span className="brief-intro-label">
                {t("brief.intro.waiting")}
              </span>
            </span>
          </span>
        )}
      </span>
      <span className="t-caption brief-intro-skip">
        {t("brief.intro.skip")}
      </span>
    </button>
  );
}

function IntroRead({
  emails,
  startedAt,
}: Readonly<{ emails: number; startedAt: number }>) {
  const { locale } = useLocale();
  const plural = usePlural();
  const delay = useCueDelay(startedAt, READ_AT_MS);
  return (
    <span className="brief-intro-read" style={{ animationDelay: `${delay}ms` }}>
      {plural("brief.intro.read", emails, {
        count: formatNumber(emails, locale),
      })}
    </span>
  );
}

function IntroFigures({
  day,
  startedAt,
}: Readonly<{ day: Worklist; startedAt: number }>) {
  const t = useT();
  const { locale } = useLocale();
  const delay = useCueDelay(startedAt, FIGURES_AT_MS);
  // A calendar that is not connected or not readable counts nothing, so its
  // zero is left out rather than shown as a measured day.
  const calendar = calendarDay(day);
  const meetings =
    calendar.state === "not_connected" || calendar.state === "unreadable"
      ? null
      : meetingsReading(day, calendar).meetings;
  const figures = [
    { label: t("brief.intro.waiting"), value: day.summary.total, tone: "" },
    {
      label: t("brief.intro.urgent"),
      value: day.summary.urgent,
      tone: day.summary.urgent > 0 ? "warning" : "",
    },
    { label: t("brief.intro.meetings"), value: meetings, tone: "" },
  ].filter(
    (figure): figure is { label: string; value: number; tone: string } =>
      typeof figure.value === "number",
  );
  return (
    <span className="brief-intro-figures">
      {figures.map((figure, index) => (
        <span
          className="brief-intro-figure"
          key={figure.label}
          style={{ animationDelay: `${delay + index * FIGURE_STEP_MS}ms` }}
        >
          <span
            className="brief-intro-value t-num"
            data-tone={figure.tone || undefined}
          >
            {formatNumber(figure.value, locale)}
          </span>
          <span className="brief-intro-label">{figure.label}</span>
        </span>
      ))}
    </span>
  );
}

/**
 * How long a part that mounts now waits for its place in the choreography.
 * A part whose read lands after its cue arrives at once instead of waiting a
 * second full delay from its own mount.
 */
function useCueDelay(startedAt: number, cueMs: number): number {
  const [delay] = useState(() => Math.max(0, cueMs - (Date.now() - startedAt)));
  return delay;
}

/** The greeting's words, each keyed by itself and how often it came before. */
function greetingWords(
  greeting: string,
): readonly { word: string; key: string }[] {
  const seen = new Map<string, number>();
  return greeting.split(" ").map((word) => {
    const count = seen.get(word) ?? 0;
    seen.set(word, count + 1);
    return { word, key: `${word}-${count}` };
  });
}
