// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useState } from "react";
import { usePrefersReducedMotion } from "../../design-system/motion";
import { formatDayLong, formatNumber, hourInZone } from "../../format/format";
import { dayInZone, viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { readStored, STORAGE_KEYS, writeStored } from "../../lib/storage";
import type { Worklist } from "../worklist/worklist.queries";
import { calendarDay } from "./brief.facts";
import { greetingText } from "./brief.glance";
import { useMorningDigest } from "./brief.queries";
import { meetingsReading } from "./brief.readings.meetings";

import "./brief.intro.css";

/** How long the greeting stands before the page shows through. */
const SHOW_MS = 4500;

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
  // A read from an earlier day would greet with yesterday's figures and spend
  // today's greeting on them, so only today's read plays it.
  const fresh =
    day !== undefined && dayInZone(Date.parse(day.as_of), zone) === today;
  const showing = due && !reduced && !ended && fresh;

  useEffect(() => {
    if (!showing) return;
    writeStored(STORAGE_KEYS.briefIntroShown, today);
    const end = () => setEnded(true);
    const timer = setTimeout(end, SHOW_MS);
    globalThis.addEventListener("keydown", end);
    return () => {
      clearTimeout(timer);
      globalThis.removeEventListener("keydown", end);
    };
  }, [showing, today]);

  if (!showing) return null;
  return (
    <IntroCurtain
      firstName={firstName}
      day={day}
      now={now}
      zone={zone}
      onEnd={() => setEnded(true)}
    />
  );
}

function IntroCurtain({
  firstName,
  day,
  now,
  zone,
  onEnd,
}: Readonly<{
  firstName: string | null;
  day: Worklist;
  now: Date;
  zone: string;
  onEnd: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
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
    { label: t("brief.intro.waiting"), value: day.summary.total },
    { label: t("brief.intro.meetings"), value: meetings },
    { label: t("brief.intro.emails"), value: emails },
  ].filter(
    (figure): figure is { label: string; value: number } =>
      typeof figure.value === "number",
  );
  return (
    // Named by its own text, so a screen reader hears the greeting and its
    // figures and not only the way out.
    <button type="button" className="brief-intro" onClick={onEnd}>
      <span className="brief-intro-body arrive-stack">
        <span className="t-eyebrow brief-intro-date">
          {formatDayLong(now.toISOString(), locale, zone)}
        </span>
        <span className="brief-intro-greeting">
          {greetingText(firstName, hourInZone(now, zone), t)}
        </span>
        <span className="brief-intro-figures">
          {figures.map((figure) => (
            <span className="brief-intro-figure" key={figure.label}>
              <span className="brief-intro-value t-num">
                {formatNumber(figure.value, locale)}
              </span>
              <span className="t-body brief-intro-label">{figure.label}</span>
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
