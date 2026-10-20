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

/**
 * The morning's greeting, over the whole page, once a local day.
 *
 * It waits for the day's read, so its figures are the page's own, and it
 * never plays for a reader who asked for less motion. Any key or press ends it
 * at once; the page under it is already drawn, so nothing waits on it.
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
  const [ended, setEnded] = useState(false);
  const [leaving, setLeaving] = useState(false);
  // A read from an earlier day would greet with yesterday's figures and spend
  // today's greeting on them, so only today's read plays it.
  const fresh =
    day !== undefined && dayInZone(Date.parse(day.as_of), zone) === today;
  const showing = due && !reduced && !ended && fresh;

  useEffect(() => {
    if (!showing) return;
    writeStored(STORAGE_KEYS.briefIntroShown, today);
    const end = () => setEnded(true);
    // Left alone it lifts away; a key or a press is a reader who wants the
    // page now, so those end it at once.
    const lift = setTimeout(() => setLeaving(true), SHOW_MS);
    const timer = setTimeout(end, SHOW_MS + LEAVE_MS);
    globalThis.addEventListener("keydown", end);
    return () => {
      clearTimeout(lift);
      clearTimeout(timer);
      globalThis.removeEventListener("keydown", end);
    };
  }, [showing, today]);

  if (!showing) return null;
  // On the body, over every layer the shell stacks, the phone's tab bar too.
  return createPortal(
    <IntroCurtain
      firstName={firstName}
      day={day}
      now={now}
      zone={zone}
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
  leaving,
  onEnd,
}: Readonly<{
  firstName: string | null;
  day: Worklist;
  now: Date;
  zone: string;
  leaving: boolean;
  onEnd: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const digest = useMorningDigest();
  // A calendar that is not connected or not readable counts nothing, so its
  // zero is left out rather than shown as a measured day.
  const calendar = calendarDay(day);
  const meetings =
    calendar.state === "not_connected" || calendar.state === "unreadable"
      ? null
      : meetingsReading(day, calendar).meetings;
  const emails = digest.data?.capture.messages_synced;
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
          <span className="brief-intro-read">
            {plural("brief.intro.read", emails, {
              count: formatNumber(emails, locale),
            })}
          </span>
        )}
        <span className="brief-intro-rule" aria-hidden="true" />
        <span className="brief-intro-figures">
          {figures.map((figure) => (
            <span className="brief-intro-figure" key={figure.label}>
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
      </span>
      <span className="t-caption brief-intro-skip">
        {t("brief.intro.skip")}
      </span>
    </button>
  );
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
