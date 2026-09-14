import { CalendarDays, ChevronUp, Clock } from "lucide-react";
import { useEffect, useId, useState } from "react";
import { Button, Modal } from "../design-system/atoms";
import { Calendar, type ISODay, isoDay } from "../design-system/calendar";
import { Eyebrow } from "../design-system/eyebrow";
import { Popover } from "../design-system/popover";
import {
  formatDateAbbrev,
  formatDateTime,
  INTL_LOCALE,
} from "../format/format";
import { viewerZone, zoneNameAndOffset } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

// Choosing WHEN a message goes, extracted from compose.tsx unchanged.
//
// It is the composer's most self-contained corner: three presets, a calendar,
// and the wall-clock arithmetic that keeps a preset meaning what it says in the
// reader's own zone. None of it touches the draft, the recipients or the send.
//
// MOVED, NOT REWRITTEN. compose.tsx sits under a frozen line cap that the next
// slices have to fit inside, and every line spent moving code is a line not
// spent on the change under review. What lands here is what was there.

function schedulePresets(now: Date): readonly {
  key: MessageKey;
  at: Date;
}[] {
  const at = (days: number, hour: number) => {
    const day = new Date(now);
    day.setDate(day.getDate() + days);
    day.setHours(hour, 0, 0, 0);
    return day;
  };
  // Monday from a Monday is NEXT Monday: a rep picking "Monday morning" on a
  // Monday afternoon means the one that has not happened yet.
  const untilMonday = (8 - now.getDay()) % 7 || 7;
  return [
    { key: "compose.scheduleTomorrow", at: at(1, 8) },
    { key: "compose.scheduleAfternoon", at: at(1, 13) },
    { key: "compose.scheduleMonday", at: at(untilMonday, 8) },
  ];
}

// The hours a business message is actually scheduled for. Four, not a clock:
// the choice is "start of the day, mid-morning, after lunch, end of the day",
// and a rep who wants 11:47 is not a rep this control is for.
const SCHEDULE_HOURS = [8, 9, 13, 17] as const;

// A moment as `datetime-local` spells it: the wall clock, no offset. Built
// field by field rather than sliced out of an ISO string, because
// `toISOString` is UTC and would move every preset by the reader's own offset.
function localMoment(at: Date): string {
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${isoDay(at)}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

/** A `datetime-local` value back as a `Date`, or nothing if the field is empty. */
export function momentOf(local: string): Date | null {
  if (!local) {
    return null;
  }
  const at = new Date(local);
  return Number.isNaN(at.getTime()) ? null : at;
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
export function ScheduleMenu({ onOpen }: Readonly<{ onOpen: () => void }>) {
  const t = useT();
  return (
    <Popover
      variant="primary"
      className="compose-sendmenu"
      label={
        <>
          <ChevronUp aria-hidden="true" size={16} />
          <span className="sr-only">{t("compose.sendOptions")}</span>
        </>
      }
    >
      <Button variant="ghost" onClick={onOpen}>
        <Clock aria-hidden="true" size={16} />
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
  onChoose,
  now,
}: Readonly<{
  open: boolean;
  onClose: () => void;
  sendAt: string;
  onChoose: (next: string) => void;
  // The clock, passed in: a component that read it itself could not be tested
  // against a fixed set of presets.
  now: Date;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const headingId = useId();
  const [picking, setPicking] = useState(false);
  const chosen = momentOf(sendAt) ?? now;
  const [day, setDay] = useState<ISODay>(isoDay(chosen));
  const [hour, setHour] = useState(chosen.getHours());
  const [month, setMonth] = useState(
    () => new Date(chosen.getFullYear(), chosen.getMonth(), 1),
  );
  // Each opening starts from the current moment, not wherever the last
  // opening left the calendar — a rep who paged to December and closed
  // without choosing should not find December still showing next time.
  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep — this re-seeds the picker on open, not on every change to sendAt/now while it's already open.
  useEffect(() => {
    if (!open) return;
    const start = momentOf(sendAt) ?? now;
    setPicking(false);
    setDay(isoDay(start));
    setHour(start.getHours());
    setMonth(new Date(start.getFullYear(), start.getMonth(), 1));
  }, [open]);
  const picked = new Date(`${day}T${String(hour).padStart(2, "0")}:00`);
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} size="wide">
      <h2 id={headingId} className="t-h2">
        {picking ? t("compose.schedulePick") : t("compose.scheduleSend")}
      </h2>
      {picking ? (
        <>
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
                    className="schedule-hour t-mono"
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
          <div className="actions">
            <Button onClick={onClose}>{t("create.cancel")}</Button>
            <Button
              variant="primary"
              onClick={() => {
                onChoose(localMoment(picked));
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
          {schedulePresets(now).map((preset) => (
            <button
              key={preset.key}
              type="button"
              className="schedule-preset"
              onClick={() => {
                onChoose(localMoment(preset.at));
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
            <CalendarDays aria-hidden="true" size={16} />
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
