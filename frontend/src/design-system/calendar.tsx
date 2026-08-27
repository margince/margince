// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A month of days, to pick one from.
//
// A native `type="date"` is the right control almost everywhere and stays the
// default (dateinput.tsx): it is the one the reader's own platform draws, it
// speaks their locale, and it costs nothing. This exists for the one case that
// control cannot serve — a date being chosen INSIDE a dialog that is already
// making a decision, where the month has to be visible while the rest of the
// choice is read. Sending a message on a Saturday is a different decision from
// sending it on the Friday, and a field that shows one day at a time cannot
// tell a reader which one they are about to pick.
//
// Presentational: the month on show and the day chosen both belong to the
// caller. A calendar that owned its own month would jump back to the selected
// day every time the dialog around it re-rendered.

import { ChevronLeft, ChevronRight } from "lucide-react";
import {
  formatNumber,
  fullDayName,
  monthAndYear,
  weekdayInitial,
} from "../format/format";
import type { Locale } from "../i18n";
import { useT } from "../i18n";
import { Button } from "./atoms";
import "./calendar.css";

/** A calendar date, as `dateinput.tsx` and the contract both spell one. */
export type ISODay = `${number}-${number}-${number}`;

/** isoDay is a `Date` as the day it falls on in the reader's own zone. */
export function isoDay(at: Date): ISODay {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}` as ISODay;
}

// The six weeks that hold any month, whichever weekday it opens on. Always six,
// never five: a grid that changed height between months made the controls under
// it move under the reader's hand as they paged.
const WEEKS = 6;

/** The days the grid draws, leading and trailing neighbours included. */
export function monthGrid(month: Date): Date[] {
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const start = new Date(first);
  start.setDate(1 - first.getDay());
  return Array.from({ length: WEEKS * 7 }, (_, index) => {
    const day = new Date(start);
    day.setDate(start.getDate() + index);
    return day;
  });
}

/**
 * Calendar is one month, with one day marked.
 *
 * `today` is passed in rather than read off the clock, so a caller can test
 * what the grid draws — and so two calendars on one screen cannot disagree
 * about what day it is.
 */
export function Calendar({
  month,
  onMonthChange,
  selected,
  onSelect,
  today,
  locale,
}: Readonly<{
  month: Date;
  onMonthChange: (next: Date) => void;
  selected: ISODay | "";
  onSelect: (next: ISODay) => void;
  today: Date;
  locale: Locale;
}>) {
  const t = useT();
  const days = monthGrid(month);
  const heading = monthAndYear(month, locale);

  return (
    <div className="calendar">
      <div className="calendar-head">
        <span className="calendar-month t-body">{heading}</span>
        <Button
          small
          iconOnly
          aria-label={t("calendar.previousMonth")}
          onClick={() => onMonthChange(shiftMonth(month, -1))}
        >
          <ChevronLeft aria-hidden="true" size={16} />
        </Button>
        <Button
          small
          iconOnly
          aria-label={t("calendar.nextMonth")}
          onClick={() => onMonthChange(shiftMonth(month, 1))}
        >
          <ChevronRight aria-hidden="true" size={16} />
        </Button>
      </div>
      <div className="calendar-grid">
        {days.slice(0, 7).map((day) => (
          // The weekday initials are a legend, not days: they carry no date
          // and announce as nothing, so the row under them reads as seven
          // dates rather than as fourteen things.
          <span
            key={`weekday-${day.getDay()}`}
            className="calendar-weekday t-caption"
            aria-hidden="true"
          >
            {weekdayInitial(day, locale)}
          </span>
        ))}
        {days.map((day) => (
          <CalendarDay
            key={isoDay(day)}
            day={day}
            inMonth={day.getMonth() === month.getMonth()}
            isToday={isoDay(day) === isoDay(today)}
            isSelected={isoDay(day) === selected}
            onSelect={onSelect}
            locale={locale}
          />
        ))}
      </div>
    </div>
  );
}

function CalendarDay({
  day,
  inMonth,
  isToday,
  isSelected,
  onSelect,
  locale,
}: Readonly<{
  day: Date;
  inMonth: boolean;
  isToday: boolean;
  isSelected: boolean;
  onSelect: (next: ISODay) => void;
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
    <button
      type="button"
      className={classes}
      // Pressed, not checked: a day is one of many and the reader is choosing
      // which one is on, which is what a pressed state says about a button.
      aria-pressed={isSelected}
      // The full date as its accessible name. "25" alone tells a screen reader
      // nothing about which month it belongs to, and the grid's own heading is
      // several stops away by then.
      aria-label={fullDayName(day, locale)}
      onClick={() => onSelect(isoDay(day))}
    >
      {formatNumber(day.getDate(), locale)}
    </button>
  );
}

// A month away, without landing in the wrong one. Set on the 1st first: from
// the 31st, `setMonth(month - 1)` walks into a month with 30 days and lands on
// the 1st of the month it started in.
function shiftMonth(month: Date, by: number): Date {
  const next = new Date(month.getFullYear(), month.getMonth(), 1);
  next.setMonth(next.getMonth() + by);
  return next;
}
