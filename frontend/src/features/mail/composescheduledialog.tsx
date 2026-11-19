// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays } from "lucide-react";
import { useId, useState } from "react";
import { Button, Modal } from "../../design-system/atoms";
import { Calendar, dateOf, type ISODay } from "../../design-system/calendar";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { calendarDay, localDateTimeValue } from "../../format/calendarday";
import {
  formatDateAbbrev,
  formatDateTime,
  INTL_LOCALE,
} from "../../format/format";
import { useNow } from "../../format/now";
import {
  instantAtWallClock,
  startOfDayInZone,
  wallClockTime,
  zoneNameAndOffset,
} from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { momentLabel, momentOf, schedulePresets } from "./composeschedule";

// The hours a business message is scheduled for. Four, not a clock: start of
// the day, mid-morning, after lunch, end of the day.
const SCHEDULE_HOURS = [8, 9, 13, 17] as const;

// A moment as `datetime-local` spells it: the wall clock, no offset. Built
// field by field rather than sliced out of an ISO string, because
// `toISOString` is UTC and would move every preset by the reader's own offset.
function localMoment(at: Date, zone: string): string {
  return localDateTimeValue(at.toISOString(), zone);
}

// When the message goes out: the three moments most sends take, then the
// calendar. One dialog, because a preset is most often the answer.
export function ScheduleDialog({
  open,
  onClose,
  sendAt,
  timezone,
  onChoose,
  now,
  sampledAt,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  sendAt: string;
  timezone?: string;
  onChoose: (next: string) => void;
  // The clock, passed in: a component that read it itself could not be tested
  // against a fixed set of presets.
  now: Date;
  /** The clock's reading when `now` was taken; absent, the dialog's first mount. */
  sampledAt?: number;
}>) {
  const viewerZone = useViewerZone();
  const headingId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <ScheduleSteps
        headingId={headingId}
        onClose={onClose}
        sendAt={sendAt}
        zone={timezone ?? viewerZone}
        onChoose={onChoose}
        now={now}
        sampledAt={sampledAt}
      />
    </Modal>
  );
}

// Modal mounts its body on each opening, so every opening starts from the
// current moment rather than wherever the last one left the calendar.
function ScheduleSteps({
  headingId,
  onClose,
  sendAt,
  zone,
  onChoose,
  now,
  sampledAt,
}: Readonly<{
  headingId: string;
  onClose: () => void;
  sendAt: string;
  zone: string;
  onChoose: (next: string) => void;
  now: Date;
  sampledAt: number | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const { current, latest } = useOpenClock(now, sampledAt);
  const [picking, setPicking] = useState(false);
  return (
    <>
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {picking ? t("compose.schedulePick") : t("compose.scheduleSend")}
      </Heading>
      {picking ? (
        <SchedulePick
          sendAt={sendAt}
          zone={zone}
          now={current}
          latest={latest}
          onChoose={onChoose}
          onClose={onClose}
        />
      ) : (
        <div className="schedule-presets">
          <p className="t-caption">
            {zoneNameAndOffset(INTL_LOCALE[locale], current, zone)}
          </p>
          {schedulePresets(current, zone).map((preset) => (
            <button
              key={preset.key}
              type="button"
              className="schedule-preset"
              onClick={() => {
                onChoose(localMoment(preset.at, zone));
                onClose();
              }}
            >
              <span className="t-body">{t(preset.key)}</span>
              <span className="t-caption">
                {formatDateTime(preset.at.toISOString(), locale, zone)}
              </span>
            </button>
          ))}
          <Button variant="ghost" onClick={() => setPicking(true)}>
            <CalendarDays aria-hidden="true" />
            {t("compose.schedulePick")}
          </Button>
          {/* Only once a moment is set. Offered over an unscheduled send it
              would be a control that undoes nothing. */}
          {sendAt !== "" && (
            <Button
              variant="ghost"
              onClick={() => {
                onChoose("");
                onClose();
              }}
            >
              {t("compose.scheduleNow")}
            </Button>
          )}
        </div>
      )}
    </>
  );
}

// The parent's `now`, moved on by the minutes the dialog stays open: an hour can
// pass before Schedule send is pressed.
// `latest` reads the clock at the press itself, between two ticks.
function useOpenClock(
  now: Date,
  sampledAt: number | undefined,
): Readonly<{
  current: Date;
  latest: () => Date;
}> {
  const tick = useNow(60_000);
  // Elapsed time runs from the sample, so a dialog whose code loads late counts
  // the wait too.
  const [opened] = useState({ at: now.getTime(), from: sampledAt ?? tick });
  // The later of the two: a parent that re-renders passes a `now` already ahead.
  const at = (clock: number) =>
    new Date(Math.max(now.getTime(), opened.at + clock - opened.from));
  return { current: at(tick), latest: () => at(Date.now()) };
}

function hourOn(day: ISODay, hour: number, zone: string): number {
  return instantAtWallClock(day, hour * 3_600_000, zone);
}

function offeredHour(
  day: ISODay,
  now: Date,
  zone: string,
): (typeof SCHEDULE_HOURS)[number] | undefined {
  return SCHEDULE_HOURS.find((hour) => hourOn(day, hour, zone) > now.getTime());
}

/** The earliest moment the picker offers: the next hour left today, else tomorrow's first. */
function firstOfferedMoment(now: Date, zone: string): Date {
  const today = calendarDay(now, zone);
  const hour = offeredHour(today, now, zone);
  if (hour !== undefined) return new Date(hourOn(today, hour, zone));
  const tomorrow = calendarDay(
    new Date(startOfDayInZone(today, zone, 1)),
    zone,
  );
  return new Date(hourOn(tomorrow, SCHEDULE_HOURS[0], zone));
}

function SchedulePick({
  sendAt,
  zone,
  now,
  latest,
  onChoose,
  onClose,
}: Readonly<{
  sendAt: string;
  zone: string;
  now: Date;
  latest: () => Date;
  onChoose: (next: string) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const hoursReasonId = useId();
  // A moment the press found already past, though the last tick had it ahead.
  const [lateAt, setLateAt] = useState<number | null>(null);
  const [start] = useState(
    () => momentOf(sendAt, zone) ?? firstOfferedMoment(now, zone),
  );
  const [day, setDay] = useState<ISODay>(calendarDay(start, zone));
  const [hour, setHour] = useState(
    Number(wallClockTime(start.getTime(), zone).split(":")[0]),
  );
  const [month, setMonth] = useState(() => {
    const month = dateOf(calendarDay(start, zone));
    month.setDate(1);
    return month;
  });
  const earliest = firstOfferedMoment(now, zone);
  const earliestDay = calendarDay(earliest, zone);
  const refusal = (candidate: ISODay) =>
    candidate < earliestDay
      ? t("calendar.earliest", {
          date: formatDateAbbrev(earliest.toISOString(), locale, zone),
        })
      : undefined;
  // A day picked after an hour keeps that hour while it is still to come.
  const pickDay = (next: ISODay) => {
    setDay(next);
    if (hourOn(next, hour, zone) > now.getTime()) return;
    const offered = offeredHour(next, now, zone);
    if (offered !== undefined) setHour(offered);
  };
  const passed = (option: number) => hourOn(day, option, zone) <= now.getTime();
  const anyPassed = SCHEDULE_HOURS.some(passed);
  const picked = new Date(hourOn(day, hour, zone));
  const past = picked.getTime() <= now.getTime() || picked.getTime() === lateAt;
  return (
    <>
      <div className="form-stack">
        <div className="schedule-pick">
          <Calendar
            month={month}
            onMonthChange={setMonth}
            selected={day}
            onSelect={pickDay}
            today={dateOf(calendarDay(now, zone))}
            locale={locale}
            refusal={refusal}
          />
          <div className="schedule-when">
            <Eyebrow>{t("compose.scheduleDate")}</Eyebrow>
            <p className="schedule-date t-body">
              {formatDateAbbrev(picked.toISOString(), locale, zone)}
            </p>
            <Eyebrow>{t("compose.scheduleTime")}</Eyebrow>
            <div className="schedule-hours">
              {SCHEDULE_HOURS.map((option) => (
                <button
                  key={option}
                  type="button"
                  className="schedule-hour t-num"
                  aria-pressed={option === hour}
                  disabled={passed(option)}
                  aria-describedby={passed(option) ? hoursReasonId : undefined}
                  onClick={() => setHour(option)}
                >
                  {`${String(option).padStart(2, "0")}:00`}
                </button>
              ))}
            </div>
            {anyPassed && (
              <p id={hoursReasonId} className="t-caption">
                {t("compose.scheduleHoursPast")}
              </p>
            )}
          </div>
        </div>
        <p className="t-caption schedule-foot">
          {t("compose.scheduleGoesOut", {
            when: momentLabel(picked, locale, zone),
          })}
        </p>
      </div>
      <div className="actions">
        <Button onClick={onClose}>{t("create.cancel")}</Button>
        <Button
          variant="primary"
          reason={past ? t("compose.schedulePast") : undefined}
          onClick={() => {
            if (picked.getTime() <= latest().getTime()) {
              setLateAt(picked.getTime());
              return;
            }
            onChoose(localMoment(picked, zone));
            onClose();
          }}
        >
          {t("compose.scheduleSend")}
        </Button>
      </div>
    </>
  );
}
