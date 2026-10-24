// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A month of days: `DateInput`'s popup, and shown open where the month must stay
// in view while the rest of a choice is read. Month and chosen day are the caller's.

import { ChevronLeft, ChevronRight } from "lucide-react";
import { type KeyboardEvent, useEffect, useId, useRef, useState } from "react";
import { isoDay } from "../format/calendarday";
import type { ISODate } from "../format/datepattern";
import {
  formatNumber,
  fullDayName,
  monthAndYear,
  weekdayInitial,
} from "../format/format";
import { INTL_LOCALE } from "../format/intllocale";
import { type Weekday, weekStart } from "../format/weekstart";
import type { Locale } from "../i18n";
import { useT } from "../i18n";
import { Button } from "./atoms";
import "./calendar.css";

export type ISODay = ISODate;

export { isoDay };

/** The local midnight a calendar day starts at. */
export function dateOf(day: ISODay): Date {
  const [year, month, date] = day.split("-").map(Number);
  return localDate(year ?? 0, (month ?? 1) - 1, date ?? 1);
}

// Not `new Date(year, month, date)`: it reads a year below 100 as 1900 onward.
function localDate(year: number, month: number, date: number): Date {
  const at = new Date(2000, 0, 1);
  at.setFullYear(year, month, date);
  return at;
}

// Always six weeks, so the controls under the grid stay put while the reader pages.
const WEEKS = 6;

/** The days the grid draws, from the week start before the 1st. */
export function monthGrid(month: Date, start: Weekday): Date[] {
  const first = localDate(month.getFullYear(), month.getMonth(), 1);
  const lead = (first.getDay() - start + 7) % 7;
  return Array.from({ length: WEEKS * 7 }, (_, index) =>
    localDate(first.getFullYear(), first.getMonth(), 1 - lead + index),
  );
}

/** One month with one day marked; `today` is the caller's, so a test can say what it draws. */
export function Calendar({
  month,
  onMonthChange,
  selected,
  onSelect,
  today,
  locale,
  refusal,
}: Readonly<{
  month: Date;
  onMonthChange: (next: Date) => void;
  selected: ISODay | "";
  onSelect: (next: ISODay) => void;
  today: Date;
  locale: Locale;
  /** Why a day cannot be chosen, translated; it joins the day's name. */
  refusal?: (day: ISODay) => string | undefined;
}>) {
  const t = useT();
  const monthId = useId();
  const start = weekStart(locale);
  const days = monthGrid(month, start);
  const [cursor, setCursor] = useState<ISODay | null>(null);
  const grid = useRef<HTMLDivElement | null>(null);
  const wantsFocus = useRef(false);
  // A drawn day outside the month can hold focus by click, so it must be the stop too.
  const shownCursor = days.some((day) => isoDay(day) === cursor)
    ? cursor
    : null;
  const tabStop = shownCursor ?? tabStopOf(month, [selected, isoDay(today)]);

  useEffect(() => {
    if (!wantsFocus.current) {
      return;
    }
    wantsFocus.current = false;
    grid.current
      ?.querySelector<HTMLElement>(`[data-day="${tabStop}"]`)
      ?.focus();
  }, [tabStop]);

  // The stop falls back to the selected day, not a stale cursor drawn in the next grid.
  const pageMonth = (by: number) => {
    setCursor(null);
    onMonthChange(shiftMonth(month, by));
  };

  const move = (event: KeyboardEvent, from: Date) => {
    // These chords belong to the browser (Back, scroll), never to the grid.
    if (event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }
    const next = stepFrom(from, event.key, event.shiftKey, start);
    if (!next) {
      return;
    }
    event.preventDefault();
    if (isoDay(next) === isoDay(from)) {
      return;
    }
    // Armed only when the tab stop will change, or no effect would clear it.
    wantsFocus.current = isoDay(next) !== tabStop;
    setCursor(isoDay(next));
    if (!sameMonth(next, month)) {
      onMonthChange(localDate(next.getFullYear(), next.getMonth(), 1));
    }
  };

  return (
    <div className="calendar">
      <div className="calendar-head">
        <span id={monthId} className="calendar-month t-body" aria-live="polite">
          {headingCase(monthAndYear(month, locale), locale)}
        </span>
        <Button
          iconOnly
          aria-label={t("calendar.previousMonth")}
          onClick={() => pageMonth(-1)}
        >
          <ChevronLeft aria-hidden="true" />
        </Button>
        <Button
          iconOnly
          aria-label={t("calendar.nextMonth")}
          onClick={() => pageMonth(1)}
        >
          <ChevronRight aria-hidden="true" />
        </Button>
      </div>
      <div className="calendar-days">
        <div className="calendar-week" aria-hidden="true">
          {days.slice(0, 7).map((day) => (
            <span key={`weekday-${day.getDay()}`}>
              {weekdayInitial(day, locale)}
            </span>
          ))}
        </div>
        {/* biome-ignore lint/a11y/useSemanticElements: not a table, so the weeks keep the legend's grid columns instead of sizing their own */}
        <div
          role="grid"
          aria-labelledby={monthId}
          className="calendar-grid"
          ref={grid}
        >
          {weeksOf(days).map((week) => (
            // biome-ignore lint/a11y/useSemanticElements lint/a11y/useFocusableInteractive: a week's day buttons take the focus, as in the date grid pattern
            <div
              role="row"
              key={isoDay(week[0] ?? month)}
              className="calendar-week"
            >
              {week.map((day) => (
                <CalendarDay
                  key={isoDay(day)}
                  day={day}
                  inMonth={sameMonth(day, month)}
                  isToday={isoDay(day) === isoDay(today)}
                  isSelected={isoDay(day) === selected}
                  isTabStop={isoDay(day) === tabStop}
                  refusal={refusal?.(isoDay(day))}
                  onSelect={onSelect}
                  onMove={move}
                  onFocusDay={setCursor}
                  locale={locale}
                />
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function CalendarDay({
  day,
  inMonth,
  isToday,
  isSelected,
  isTabStop,
  refusal,
  onSelect,
  onMove,
  onFocusDay,
  locale,
}: Readonly<{
  day: Date;
  inMonth: boolean;
  isToday: boolean;
  isSelected: boolean;
  isTabStop: boolean;
  refusal: string | undefined;
  onSelect: (next: ISODay) => void;
  onMove: (event: KeyboardEvent, from: Date) => void;
  onFocusDay: (day: ISODay) => void;
  locale: Locale;
}>) {
  const classes = [
    "calendar-day",
    inMonth ? "" : "calendar-day-outside",
    isToday ? "calendar-day-today" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    // biome-ignore lint/a11y/useSemanticElements lint/a11y/useFocusableInteractive: the day button inside takes the focus, as in the date grid pattern
    <div role="gridcell" aria-selected={isSelected}>
      <button
        type="button"
        className={classes}
        data-day={isoDay(day)}
        tabIndex={isTabStop ? 0 : -1}
        aria-current={isToday ? "date" : undefined}
        aria-label={
          refusal
            ? `${fullDayName(day, locale)}, ${refusal}`
            : fullDayName(day, locale)
        }
        // Not `disabled`: a disabled day takes no focus, so the arrows would skip it.
        aria-disabled={refusal === undefined ? undefined : true}
        // The tab stop follows focus, so a click on one day never leaves another as the stop.
        onFocus={() => onFocusDay(isoDay(day))}
        onKeyDown={(event) => onMove(event, day)}
        onClick={() => {
          if (refusal === undefined) {
            onSelect(isoDay(day));
          }
        }}
      >
        {formatNumber(day.getDate(), locale)}
      </button>
    </div>
  );
}

function tabStopOf(
  month: Date,
  candidates: ReadonlyArray<ISODay | "" | null>,
): ISODay {
  const inMonth = candidates.find(
    (day): day is ISODay => !!day && sameMonth(dateOf(day), month),
  );
  return inMonth ?? isoDay(localDate(month.getFullYear(), month.getMonth(), 1));
}

function stepFrom(
  from: Date,
  key: string,
  shift: boolean,
  start: Weekday,
): Date | null {
  const by = (days: number) =>
    localDate(from.getFullYear(), from.getMonth(), from.getDate() + days);
  const intoWeek = (from.getDay() - start + 7) % 7;
  switch (key) {
    case "ArrowLeft":
      return by(-1);
    case "ArrowRight":
      return by(1);
    case "ArrowUp":
      return by(-7);
    case "ArrowDown":
      return by(7);
    case "Home":
      return by(-intoWeek);
    case "End":
      return by(6 - intoWeek);
    case "PageUp":
      return sameDayMonthsAway(from, shift ? -12 : -1);
    case "PageDown":
      return sameDayMonthsAway(from, shift ? 12 : 1);
    default:
      return null;
  }
}

// The 31st a month away lands on that month's last day, not in the month after.
function sameDayMonthsAway(from: Date, by: number): Date {
  const target = localDate(from.getFullYear(), from.getMonth() + by, 1);
  const last = localDate(
    target.getFullYear(),
    target.getMonth() + 1,
    0,
  ).getDate();
  return localDate(
    target.getFullYear(),
    target.getMonth(),
    Math.min(from.getDate(), last),
  );
}

function sameMonth(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth();
}

function shiftMonth(month: Date, by: number): Date {
  return localDate(month.getFullYear(), month.getMonth() + by, 1);
}

function weeksOf(days: readonly Date[]): Date[][] {
  return Array.from({ length: WEEKS }, (_, week) =>
    days.slice(week * 7, week * 7 + 7),
  );
}

// Vietnamese writes a month name in lower case; a heading opens with a capital.
function headingCase(text: string, locale: Locale): string {
  const [first = "", ...rest] = [...text];
  return first.toLocaleUpperCase(INTL_LOCALE[locale]) + rest.join("");
}
