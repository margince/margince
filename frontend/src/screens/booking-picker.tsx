import { ChevronLeft, ChevronRight } from "lucide-react";
import type { components } from "../api/schema";
import { Button, SegmentedControl } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { IconAction } from "../design-system/iconaction";
import { type MeetingDay, MeetingWeek } from "../design-system/meetingslots";
import { Panel, PanelBody } from "../design-system/panel";
import { stable } from "../format/collate";
import { dateTileParts } from "../format/datetile";
import {
  formatDate,
  formatDateTime,
  formatTimeOfDay,
  identifierNumber,
} from "../format/format";
import { dayInZone, startOfDayInZone } from "../format/timezone";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { useInviteAvailability } from "./booking-availability";
import { BookingZone } from "./booking-common";
import { QueryGate } from "./common";
import { minutesOf, type useWorkingHours } from "./working-hours";

export type BookingMode = "propose" | "invite" | "link";
export type BookingSlot = { start: string; end: string };

const DAY = 86400000;
const LENGTHS = ["15", "30", "45", "60"];

type Slot = components["schemas"]["MeetingAvailability"]["slots"][number];
type WorkingHours = components["schemas"]["WorkingHours"];

const isoWeekday = (day: string) =>
  new Date(`${day}T12:00:00Z`).getUTCDay() || 7;

// Whether a day shown in `zone` meets the host's working window, which is kept
// on the host's own clock: across zones one shown day spans two host days.
function overlapsWorkingDay(day: string, zone: string, hours: WorkingHours) {
  const start = Date.parse(startOfDayInZone(day, zone));
  const end = start + DAY;
  const hostDays = new Set(
    [start, end - 1].map((instant) => dayInZone(instant, hours.timezone)),
  );
  return [...hostDays].some((hostDay) => {
    if (!hours.days.includes(isoWeekday(hostDay))) return false;
    const midnight = Date.parse(startOfDayInZone(hostDay, hours.timezone));
    const open = midnight + minutesOf(hours.start_time) * 60000;
    const close = midnight + minutesOf(hours.end_time) * 60000;
    return open < end && close > start;
  });
}

// The columns a week shows: every day that has a free time, plus the host's
// working days that have none, so an empty Tuesday reads as busy rather than
// as missing.
export function weekDays(
  slots: readonly Slot[],
  from: string,
  windowDays: number,
  hours: WorkingHours | undefined,
  locale: Locale,
  zone: string,
): MeetingDay[] {
  const byDay = new Map<string, Slot[]>();
  const start = Date.parse(from);
  for (let offset = 0; offset < windowDays; offset++) {
    const key = dayInZone(start + offset * DAY, zone);
    if (!hours || overlapsWorkingDay(key, zone, hours)) byDay.set(key, []);
  }
  for (const slot of slots) {
    const key = dayInZone(Date.parse(slot.start), zone);
    byDay.set(key, [...(byDay.get(key) ?? []), slot]);
  }
  return [...byDay.entries()]
    .sort(([a], [b]) => stable(a, b))
    .map(([key, daySlots]) => {
      const noon = Date.parse(startOfDayInZone(key, zone)) + DAY / 2;
      const tile = dateTileParts(new Date(noon).toISOString(), locale, zone);
      return {
        key,
        weekday: tile.weekday,
        date: `${tile.day} ${tile.month}`,
        slots: daySlots.map((slot) => ({
          ...slot,
          label: formatDateTime(slot.start, locale, zone),
          time: formatTimeOfDay(slot.start, locale, zone),
        })),
      };
    });
}

const PICKER_TITLE = {
  propose: "scheduling.pickOffer",
  invite: "scheduling.pickAgreed",
  link: "scheduling.openTimes",
} as const;

export function BookingPicker({
  mode,
  configured,
  profile,
  hours,
  zone,
  onZone,
  duration,
  onDuration,
  from,
  onFrom,
  searchAhead,
  onSearchAhead,
  picks,
  onPick,
}: Readonly<{
  mode: BookingMode;
  configured: boolean;
  profile: components["schemas"]["SchedulingProfile"] | undefined;
  hours: ReturnType<typeof useWorkingHours>;
  zone: string;
  onZone: (zone: string) => void;
  duration: number;
  onDuration: (minutes: number) => void;
  from: string;
  onFrom: (from: string) => void;
  searchAhead: boolean;
  onSearchAhead: (on: boolean) => void;
  picks: readonly BookingSlot[];
  onPick: (slot: BookingSlot) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { earliest, latest, slots } = useInviteAvailability(
    from,
    duration,
    searchAhead,
    configured,
    profile,
    hours.data,
  );
  const start = Date.parse(from);
  const lengths = LENGTHS.includes(identifierNumber(duration))
    ? LENGTHS
    : [...LENGTHS, identifierNumber(duration)].sort(
        (a, b) => Number(a) - Number(b),
      );
  const goTo = (at: number) => {
    onFrom(
      at <= Date.now()
        ? new Date().toISOString()
        : startOfDayInZone(dayInZone(at, zone), zone),
    );
    onSearchAhead(false);
  };
  return (
    <Panel title={t(PICKER_TITLE[mode])}>
      <PanelBody className="book-picker">
        <div className="book-picker-bar">
          <IconAction
            label={t("scheduling.previousWeek")}
            icon={<ChevronLeft aria-hidden />}
            disabled={start <= earliest}
            onClick={() => goTo(start - 7 * DAY)}
          />
          <span className="t-name">
            {formatDate(new Date(start).toISOString(), locale, zone)} –{" "}
            {formatDate(new Date(start + 6 * DAY).toISOString(), locale, zone)}
          </span>
          <IconAction
            label={t("scheduling.nextWeek")}
            icon={<ChevronRight aria-hidden />}
            disabled={start + 7 * DAY >= latest}
            onClick={() => goTo(start + 7 * DAY)}
          />
          <SegmentedControl
            label={t("scheduling.length")}
            options={lengths}
            value={identifierNumber(duration)}
            labels={Object.fromEntries(
              lengths.map((minutes) => [
                minutes,
                plural("scheduling.minutes", Number(minutes), {
                  count: minutes,
                }),
              ]),
            )}
            onChange={(minutes) => onDuration(Number(minutes))}
          />
        </div>
        <BookingZone value={zone} onChange={onZone} />
        {!configured ? (
          <p className="t-caption">{t("scheduling.finishSetup")}</p>
        ) : (
          <QueryGate pendingLabel={t("common.loading")} query={slots}>
            {(value) => (
              <>
                <MeetingWeek
                  days={weekDays(
                    value.slots,
                    from,
                    searchAhead ? 0 : 7,
                    hours.data?.working_hours,
                    locale,
                    zone,
                  )}
                  selected={
                    mode === "link" ? [] : picks.map((pick) => pick.start)
                  }
                  onSelect={mode === "link" ? undefined : onPick}
                  emptyDay={t("scheduling.noFreeTime")}
                />
                {value.slots.length === 0 && searchAhead && (
                  <p className="t-caption">{t("scheduling.noTimesHorizon")}</p>
                )}
                <div className="book-picker-foot">
                  <p className="t-caption">
                    {t("scheduling.bookingUntil", {
                      date: formatDate(
                        new Date(latest).toISOString(),
                        locale,
                        zone,
                      ),
                    })}{" "}
                    {t("scheduling.noTimesHelp")}
                  </p>
                  {!searchAhead && (
                    <Button variant="link" onClick={() => onSearchAhead(true)}>
                      {t("scheduling.findNext")}
                    </Button>
                  )}
                </div>
              </>
            )}
          </QueryGate>
        )}
        <ErrorLine error={hours.error} />
      </PanelBody>
    </Panel>
  );
}

export function BookingBlocked() {
  const t = useT();
  return (
    <Panel title={t("scheduling.setupCalendar")}>
      <PanelBody className="book-picker">
        <Heading as="h2" size="small">
          {t("scheduling.connectFirst")}
        </Heading>
        <p>{t("scheduling.setupInSettings")}</p>
        <a href="#/settings/meetings" target="_blank" rel="noreferrer">
          {t("scheduling.openSettings")}
        </a>
      </PanelBody>
    </Panel>
  );
}
