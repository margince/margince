import { CalendarDays, ChevronUp, Clock } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type { components } from "../../api/schema";
import { Button, Modal } from "../../design-system/atoms";
import { Calendar, dateOf, type ISODay } from "../../design-system/calendar";
import { Eyebrow } from "../../design-system/eyebrow";
import { Heading } from "../../design-system/heading";
import { Popover } from "../../design-system/popover";
import {
  calendarDay,
  isRealCalendarDay,
  localDateTimeValue,
} from "../../format/calendarday";
import {
  formatDateAbbrev,
  formatDateTime,
  INTL_LOCALE,
} from "../../format/format";
import {
  browserZone,
  instantAtWallClock,
  startOfDayInZone,
  wallClockTime,
  zoneNameAndOffset,
} from "../../format/timezone";
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useViewerZone } from "../../lib/viewerzone";

// Choosing WHEN a message goes, extracted from compose.tsx unchanged.
//
// It is the composer's most self-contained corner: three presets, a calendar,
// and the wall-clock arithmetic that keeps a preset meaning what it says in the
// reader's own zone. None of it touches the draft, the recipients or the send.
//
// MOVED, NOT REWRITTEN. compose.tsx sits under a frozen line cap that the next
// slices have to fit inside, and every line spent moving code is a line not
// spent on the change under review. What lands here is what was there.

export function schedulePresets(
  now: Date,
  zone = browserZone(),
): readonly {
  key: MessageKey;
  at: Date;
}[] {
  const today = calendarDay(now, zone);
  const at = (days: number, hour: number) => {
    const day = calendarDay(
      new Date(startOfDayInZone(today, zone, days)),
      zone,
    );
    return new Date(instantAtWallClock(day, hour * 3_600_000, zone));
  };
  const weekday = new Intl.DateTimeFormat("en-US", {
    weekday: "short",
    timeZone: zone,
  }).format(now);
  const index = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].indexOf(
    weekday,
  );
  const untilMonday = (8 - index) % 7 || 7;
  return [
    { key: "compose.scheduleTomorrow", at: at(1, 8) },
    { key: "compose.scheduleAfternoon", at: at(1, 13) },
    { key: "compose.scheduleMonday", at: at(untilMonday, 8) },
  ];
}

type ScheduleSource = Pick<
  components["schemas"]["SendEmailRequest"],
  "scheduled_at" | "scheduled_tz" | "scheduled_intent"
>;
export type ScheduleFields = {
  [Key in keyof ScheduleSource]?: NonNullable<ScheduleSource[Key]>;
};

// A saved legacy instant remains an instant until the reader chooses a time.
export function scheduleFields(
  local: string,
  saved?: ScheduleFields,
  timezone = browserZone(),
): ScheduleFields {
  if (local === "") return {};
  if (saved !== undefined) return saved;
  const at = scheduleInstant(local, timezone);
  return {
    scheduled_at: at.toISOString(),
    scheduled_tz: timezone,
    scheduled_intent: { wall_clock: local, timezone },
  };
}

function scheduleInstant(local: string, timezone: string): Date {
  const parts =
    /^([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2}):([0-9]{2})(?::([0-9]{2}(?:\.[0-9]{1,6})?))?$/.exec(
      local,
    );
  if (!parts || !isRealCalendarDay(parts[1] ?? ""))
    throw new RangeError("Invalid scheduled time");
  const day = parts[1] ?? "";
  const hour = Number(parts[2]);
  const minute = Number(parts[3]);
  const second = Number(parts[4] ?? 0);
  if (hour > 23 || minute > 59 || second >= 60)
    throw new RangeError("Invalid scheduled time");
  const milliseconds = (hour * 3600 + minute * 60 + Math.floor(second)) * 1000;
  let instant: number;
  try {
    instant = instantAtWallClock(day, milliseconds, timezone);
  } catch (error) {
    if (!(error instanceof RangeError) || Number(day.substring(0, 4)) < 100)
      throw error;
    instant = Date.parse(startOfDayInZone(day, timezone, 1));
  }
  return new Date(instant);
}

export function scheduleSourceOf(saved: ScheduleSource): ScheduleFields {
  return {
    scheduled_at: saved.scheduled_at ?? undefined,
    scheduled_tz: saved.scheduled_tz ?? undefined,
    scheduled_intent: saved.scheduled_intent ?? undefined,
  };
}

export function sameScheduleSource(
  a?: ScheduleFields,
  b?: ScheduleFields,
): boolean {
  return (
    a?.scheduled_at === b?.scheduled_at &&
    a?.scheduled_tz === b?.scheduled_tz &&
    a?.scheduled_intent?.wall_clock === b?.scheduled_intent?.wall_clock &&
    a?.scheduled_intent?.timezone === b?.scheduled_intent?.timezone
  );
}

// The hours a business message is actually scheduled for. Four, not a clock:
// the choice is "start of the day, mid-morning, after lunch, end of the day",
// and a rep who wants 11:47 is not a rep this control is for.
const SCHEDULE_HOURS = [8, 9, 13, 17] as const;

// A moment as `datetime-local` spells it: the wall clock, no offset. Built
// field by field rather than sliced out of an ISO string, because
// `toISOString` is UTC and would move every preset by the reader's own offset.
function localMoment(at: Date, zone = browserZone()): string {
  return localDateTimeValue(at.toISOString(), zone);
}

export function momentOf(local: string, zone = browserZone()): Date | null {
  if (!local) return null;
  try {
    return scheduleInstant(local, zone);
  } catch (error) {
    if (error instanceof RangeError) return null;
    throw error;
  }
}

/** A scheduled moment, spelled the way every line in this drawer spells one. */
export function momentLabel(at: Date, locale: Locale, zone: string): string {
  return `${formatDateTime(at.toISOString(), locale, zone)} · ${zoneNameAndOffset(INTL_LOCALE[locale], at)}`;
}

// The other way to send, behind the confirm button's own caret.
//
// A moment is a choice ABOUT the send rather than a field of the message, so
// it belongs with the send control and not in the form above it — it was a
// datetime field between the consent purpose and the recipient warnings, which
// is where a rep reads the message rather than where they decide to release
// it.
export function ScheduleMenu({
  onOpen,
  reasonId,
}: Readonly<{
  onOpen: () => void;
  /** The sentence that holds Send holds this menu too. */
  reasonId?: string;
}>) {
  const t = useT();
  return (
    <Popover
      variant="primary"
      reasonId={reasonId}
      className="compose-sendmenu"
      label={
        <>
          <ChevronUp aria-hidden="true" size={16} />
          <span className="sr-only">{t("compose.sendOptions")}</span>
        </>
      }
    >
      <Button variant="ghost" onClick={onOpen}>
        <Clock aria-hidden="true" />
        {t("compose.scheduleSend")}
      </Button>
    </Popover>
  );
}

// Choosing when the message goes out: the three moments most sends take, and
// the calendar for the ones that do not.
//
// Two steps in one dialog rather than two dialogs. The presets ARE the answer
// most of the time, and a rep who wants one of them should not have to walk a
// calendar to reach it; a rep who wants a different Thursday needs the month
// in front of them. One dialog keeps that as one decision.
export function ScheduleDialog({
  open,
  onClose,
  sendAt,
  timezone,
  onChoose,
  now,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  sendAt: string;
  timezone?: string;
  onChoose: (next: string) => void;
  // The clock, passed in: a component that read it itself could not be tested
  // against a fixed set of presets.
  now: Date;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const viewerZone = useViewerZone();
  const zone = timezone ?? viewerZone;
  const headingId = useId();
  const [picking, setPicking] = useState(false);
  const chosen = momentOf(sendAt, zone) ?? now;
  const [day, setDay] = useState<ISODay>(calendarDay(chosen, zone));
  const [hour, setHour] = useState(
    Number(wallClockTime(chosen.getTime(), zone).split(":")[0]),
  );
  const [month, setMonth] = useState(() => {
    const month = dateOf(calendarDay(chosen, zone));
    month.setDate(1);
    return month;
  });
  // Each opening starts from the current moment, not wherever the last
  // opening left the calendar — a rep who paged to December and closed
  // without choosing should not find December still showing next time.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep — this re-seeds the picker on open, not on every change to sendAt/now while it's already open.
  useEffect(() => {
    if (!open) return;
    const start = momentOf(sendAt, zone) ?? now;
    setPicking(false);
    setDay(calendarDay(start, zone));
    setHour(Number(wallClockTime(start.getTime(), zone).split(":")[0]));
    const month = dateOf(calendarDay(start, zone));
    month.setDate(1);
    setMonth(month);
  }, [open]);
  const picked = new Date(instantAtWallClock(day, hour * 3_600_000, zone));
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="form">
      <Heading size="large" id={headingId} className="t-h2 modal-title">
        {picking ? t("compose.schedulePick") : t("compose.scheduleSend")}
      </Heading>
      {picking ? (
        <>
          <div className="form-stack">
            <div className="schedule-pick">
              <Calendar
                month={month}
                onMonthChange={setMonth}
                selected={day}
                onSelect={setDay}
                today={now}
                locale={locale}
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
                      onClick={() => setHour(option)}
                    >
                      {`${String(option).padStart(2, "0")}:00`}
                    </button>
                  ))}
                </div>
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
              onClick={() => {
                onChoose(localMoment(picked, zone));
                onClose();
              }}
            >
              {t("compose.scheduleSend")}
            </Button>
          </div>
        </>
      ) : (
        <div className="schedule-presets">
          <p className="t-caption">
            {zoneNameAndOffset(INTL_LOCALE[locale], now)}
          </p>
          {schedulePresets(now, zone).map((preset) => (
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
    </Modal>
  );
}
