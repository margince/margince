// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three zones this product renders times in, and the rule for which one a
// screen owes its reader. `formatDate`/`formatDateTime`/`formatDateAbbrev` take
// their zone as an argument and only attach it (format.ts, zone-by-purpose).
// Picking the zone is the caller's judgement, and this file is where that
// judgement is written down once.
//
//   useViewerZone() — the signed-in user's own clock (lib/viewerzone.tsx):
//   the zone they chose under Settings → Account, or their browser's until they
//   have one. EVERY time a signed-in screen shows is read in it: activities,
//   notes, captured mail, tasks, meetings, approvals, history and Home. A note
//   saved at 16:03 in Berlin shows 16:03 to a reader in Berlin, whatever zone
//   the installation uses. This is the default; the two below are exceptions.
//
//   useCompanyZone() — the company's own clock (`installation.timezone`, served
//   by lib/companyzone.tsx). It is for what belongs to the company: report
//   periods and runs, and the days and weeks the server buckets on the
//   company's clock (the morning brief's day, the weekly's week, the server's
//   "today" and "overdue" lists). Naming those in another zone would label a
//   bucket with days it does not hold.
//
//   browserZone() — the browser's own clock. Only for screens with no
//   signed-in reader (public booking, confirm, the buyer room, sign-in), for
//   the reader's fallback, and for a datetime-local input, which the browser
//   parses in its own zone.
//
// The census in zone-by-purpose.test.ts lists every file that reads the
// company or the browser zone, each with its reason. A file not on that list
// reads the reader's zone.
//
// A date-only wire value (OpenAPI `format: date`) has no instant to localize.
// The formatters read it at its midday in the zone they are given, so it names
// the same calendar day in every zone a user can live in.
//
// A task's due day is set in the zone of the user who picks it (decision A1
// on #7894): `dueInstant` mints the picked day's end in the picker's own zone,
// and every reader then sees that moment in theirs. Tasks stored before this
// were minted in the company zone and keep that moment.
//
// It is a HOOK rather than a constant for both stored zones, because each
// answer arrives over the wire. A plain helper that needs one takes it as a
// parameter from the component that read it — never by importing the fallback
// below, which is a different value that only happens to agree.

import { isRealCalendarDay } from "./calendarday";

// The zone a record's dates are read in when the installation's own answer is
// missing. It is only the last resort. `useCompanyZone` in
// lib/companyzone.tsx serves `installation.timezone`. The authenticated shell
// holds its first paint until that read lands, so no signed-in surface renders
// a record date against this constant.
//
// It stays a real zone rather than UTC because the surfaces that can still
// reach it are the ones outside the settings read — a story, a test that mounts
// a screen bare — and those are easier to read against a zone with an offset
// and a DST rule than against one with neither.
//
// Do not import it into a screen. The gate in zone-by-purpose.test.ts refuses
// that, and names this comment when it does.
export const UTC_ZONE = "UTC";

export const FALLBACK_COMPANY_ZONE = "Europe/Berlin";

// IANA zone names only (AC-DS-TZ4). Fixed offsets ("+01:00", "Etc/GMT-1")
// freeze DST rules without a sign, so they throw at the edge.
export function assertIanaZone(zone: string): void {
  if (/^[+-]\d{2}:?\d{2}$/.test(zone) || /^(Etc\/)?GMT[+-]?\d*$/i.test(zone)) {
    throw new Error(
      `timezone must be an IANA name, got fixed offset "${zone}"`,
    );
  }
  // Intl rejects unknown names: constructing with the zone throws a RangeError
  // that propagates. format() forces the value to be consumed.
  new Intl.DateTimeFormat("en-US", { timeZone: zone }).format();
}

/**
 * Whether the formatters will accept this zone, asked without throwing.
 *
 * A page rendering a list of moments cannot let one row's zone take it down,
 * so it asks before it formats. A probe with Intl alone learns only that the
 * name resolves. `Etc/GMT-1`, `GMT` and `+01:00` resolve, then throw inside
 * `formatDate`. So this derives from the assertion, the one reading of
 * "a zone this renders".
 */
export function isRenderableZone(zone: string): boolean {
  try {
    assertIanaZone(zone);
    return true;
  } catch {
    return false;
  }
}

/**
 * The reader's own zone, named and offset — "Indochina Time · UTC+7".
 *
 * Every line about a moment a message will be RELEASED at carries it. A send
 * scheduled from Ho Chi Minh City for an account in Munich is a moment in ONE
 * of those places, and "Tuesday, 08:00" with no zone is the sentence that gets
 * that wrong.
 *
 * A named `zone` reads that zone instead of the reader's own.
 *
 * Both halves come from Intl's own names, so the zone reads in the reader's
 * language; the offset is spelled UTC rather than Intl's GMT, because that is
 * what the rest of the product says.
 */
export function zoneNameAndOffset(
  locale: string,
  at: Date,
  zone?: string,
): string {
  // A zone the formatters refuse is shown as stored, not dropped.
  if (zone !== undefined && !isRenderableZone(zone)) return zone;
  const named = (style: "long" | "shortOffset") =>
    new Intl.DateTimeFormat(locale, { timeZone: zone, timeZoneName: style })
      .formatToParts(at)
      .find((piece) => piece.type === "timeZoneName")?.value ?? "";
  return [named("long"), named("shortOffset").replace("GMT", "UTC")]
    .filter(Boolean)
    .join(" · ");
}

// A function, so a change of zone is read on the next call. A runtime that
// reports no zone gets UTC, since Intl throws on an empty one.
export function browserZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || UTC_ZONE;
}

// The wall clock `zone` reads at the instant `utcMs` names, spelled as if it
// were a UTC instant, so two readings compare and subtract as plain numbers.
// Read to the second: Intl never reports a fraction.
function wallClockMs(utcMs: number, zone: string): number {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      hourCycle: "h23",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    })
      .formatToParts(new Date(utcMs))
      .map((part) => [part.type, part.value]),
  );
  return Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour),
    Number(parts.minute),
    Number(parts.second),
  );
}

// The offset (ms) `zone` sits at relative to UTC at the instant `utcMs`
// names: positive east of UTC, negative west. It is the zone's wall clock
// read back against UTC, which needs no timezone-database library.
export function zoneOffsetMs(utcMs: number, zone: string): number {
  // The wall clock is read to the second, so the instant is too: compared
  // with its milliseconds, an instant 1ms past a second read 1ms off UTC.
  return wallClockMs(utcMs, zone) - Math.floor(utcMs / 1000) * 1000;
}

// instantInZone resolves the UTC instant at which `zone`'s wall clock reads
// the given date and time. Two passes: the zone's offset can itself change
// within the correction (a DST transition landing exactly at that wall-clock
// moment) — re-deriving it from the corrected instant resolves that edge.
// Whole seconds only: Intl.DateTimeFormat never reports fractional seconds,
// so a sub-second component would desync the wall-clock comparison from the
// reconstructed one by up to a second.
function instantInZone(
  dateOnly: string,
  zone: string,
  hour: number,
  minute: number,
  second: number,
): number {
  const [year = Number.NaN, month = Number.NaN, day = Number.NaN] = dateOnly
    .split("-")
    .map(Number);
  const wallClockMs = Date.UTC(year, month - 1, day, hour, minute, second);
  let utcMs = wallClockMs;
  for (let pass = 0; pass < 2; pass += 1) {
    utcMs = wallClockMs - zoneOffsetMs(utcMs, zone);
  }
  return utcMs;
}

// The calendar day `zone`'s wall clock reads at `utcMs`, as `yyyy-mm-dd`.
// Derived through Intl rather than by arithmetic on the offset, because the
// offset is what is in question at the moments this is asked about.
export function dayInZone(utcMs: number, zone: string): string {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat("en-US", {
      timeZone: zone,
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    })
      .formatToParts(new Date(utcMs))
      .map((part) => [part.type, part.value]),
  );
  return `${parts.year}-${parts.month}-${parts.day}`;
}

/**
 * The first instant of `dateOnly` in `zone` — the day's true beginning, even
 * when its midnight does not exist.
 *
 * A spring-forward transition can REMOVE a wall-clock hour, and some zones put
 * that transition at midnight: Santiago goes from 23:00 on 5 September 2026
 * straight to 01:00 on the 6th, so `2026-09-06T00:00` is a time that never
 * happens there. Asked for it, `instantInZone` converges on the instant one
 * hour before the jump, whose local reading is still 23:00 on the FIFTH — a
 * "from 6 September" filter then pulls in an hour of the fifth, and an
 * exclusive "to the fifth" end drops that hour instead.
 *
 * So the resolved instant is checked against the day it was supposed to land
 * on. When it falls short, the day's first instant is the transition itself,
 * found by stepping forward a minute at a time — a removed span is a whole
 * number of minutes in every zone tzdata describes, and the largest is
 * Lord Howe's 30, so the walk is bounded and short. Stepping BACK is never
 * needed: the resolved instant is never later than the day's start, because a
 * removed hour only ever moves the clock forward.
 */
function startOfDayInstant(dateOnly: string, zone: string): number {
  const resolved = instantInZone(dateOnly, zone, 0, 0, 0);
  if (dayInZone(resolved, zone) === dateOnly) {
    return resolved;
  }
  // No minute in two hours reads as the wanted day. That is not a zone rule
  // this code knows how to interpret, and guessing would silently file records
  // under a day nobody picked — so the honest answer is the unadjusted one,
  // which is what every caller got before this correction existed.
  return (
    firstMinuteReadingAtLeast(wallClockOfDay(dateOnly, 0), zone, resolved) ??
    resolved
  );
}

const MINUTE_MS = 60_000;
export const DAY_MS = 86_400_000;

// The first whole minute after `from` at which `zone`'s wall clock reads
// `wall` or later: the end of a removed span. Two hours covers every
// transition tzdata records, so the walk gives up after 120 steps.
function firstMinuteReadingAtLeast(
  wall: number,
  zone: string,
  from: number,
): number | undefined {
  const start = Math.floor(from / MINUTE_MS) * MINUTE_MS;
  for (let step = 1; step <= 120; step += 1) {
    const candidate = start + step * MINUTE_MS;
    if (wallClockMs(candidate, zone) >= wall) {
      return candidate;
    }
  }
  return undefined;
}

// A `yyyy-mm-dd` day plus a time of day, in the wallClockMs spelling.
function wallClockOfDay(dateOnly: string, timeOfDayMs: number): number {
  const [year = Number.NaN, month = Number.NaN, day = Number.NaN] = dateOnly
    .split("-")
    .map(Number);
  return Date.UTC(year, month - 1, day) + timeOfDayMs;
}

/**
 * The time of day a task is due at, as `HH:MM` on `zone`'s wall clock, or
 * `""` for a task due by a day. `hasTime` is the task's own `due_has_time`:
 * the server's flag decides, never what the clock happens to read, so a task
 * stored at any instant of its day reads as a day until it says otherwise.
 */
export function dueTimeOf(
  dueAt: string,
  hasTime: boolean | undefined,
  zone: string,
): string {
  const at = Date.parse(dueAt);
  if (hasTime !== true || Number.isNaN(at)) {
    return "";
  }
  return wallClockTime(at, zone);
}

/** `HH:MM` on `zone`'s wall clock at the instant `at`. */
export function wallClockTime(at: number, zone: string): string {
  const timeOfDay = ((wallClockMs(at, zone) % DAY_MS) + DAY_MS) % DAY_MS;
  const minutes = Math.floor(timeOfDay / 60_000);
  const pad = (value: number) => String(value).padStart(2, "0");
  return `${pad(Math.floor(minutes / 60))}:${pad(minutes % 60)}`;
}

/**
 * The instant on `day` at which `zone`'s wall clock reads `timeOfDayMs` past
 * midnight, in whole seconds. On a DST switch day a wall time can be missing
 * or happen twice:
 * - missing (02:30 in Berlin on 29 March 2026): the first instant after the
 *   gap, 03:00;
 * - twice (02:30 in Berlin on 25 October 2026): the earlier one, on summer time.
 *
 * A day the zone skipped whole (30 December 2011 in Apia) has no such instant,
 * so it is refused rather than filed on a neighbouring day.
 */
export function instantAtWallClock(
  day: string,
  timeOfDayMs: number,
  zone: string,
): number {
  if (!isRealCalendarDay(day)) {
    throw new RangeError(`not a calendar day: ${day}`);
  }
  // Date.UTC maps years 0-99 onto 1900-1999, so such a day would resolve to
  // another century without a sign.
  if (Number(day.slice(0, 4)) < 100) {
    throw new RangeError(`year out of range: ${day}`);
  }
  const wanted = wallClockOfDay(day, timeOfDayMs);
  // The offsets a day either side of the wanted time. On a switch day they
  // are the two in force; on any other day they agree.
  const offsets = [
    zoneOffsetMs(wanted - DAY_MS, zone),
    zoneOffsetMs(wanted + DAY_MS, zone),
  ];
  const readings = offsets
    .map((offset) => wanted - offset)
    .filter((candidate) => wallClockMs(candidate, zone) === wanted);
  if (readings.length > 0) {
    return Math.min(...readings);
  }
  const afterGap = firstMinuteReadingAtLeast(
    wanted,
    zone,
    wanted - Math.max(...offsets),
  );
  if (afterGap === undefined || dayInZone(afterGap, zone) !== day) {
    throw new RangeError(`${day} has no such time in ${zone}`);
  }
  return afterGap;
}

/**
 * The instant on `day` at which `zone`'s wall clock reads what it read at
 * `instant`. A meeting moved to another day keeps its time of day there, by
 * the DST rule of `instantAtWallClock`. The source's milliseconds are kept,
 * because the wall clock is read to the second.
 */
export function sameWallClockOnDay(
  instant: string,
  day: string,
  zone: string,
): string {
  const sourceMs = Date.parse(instant);
  if (Number.isNaN(sourceMs)) {
    throw new RangeError(`not an instant: ${instant}`);
  }
  const fraction = sourceMs - Math.floor(sourceMs / 1000) * 1000;
  const sourceWall = wallClockMs(sourceMs, zone);
  const timeOfDay = ((sourceWall % DAY_MS) + DAY_MS) % DAY_MS;
  return new Date(
    instantAtWallClock(day, timeOfDay, zone) + fraction,
  ).toISOString();
}

/**
 * A calendar day, picked as a date-only string, read as a day in `zone`.
 * Minting it as `new Date(dateOnly).toISOString()` reads the string as UTC
 * midnight, which silently rolls the day back for anyone west of UTC and
 * forward for evening messages east of it: the picker and the rendered row
 * would then disagree about which day was meant.
 *
 * `startOfDayInZone` is the day's first instant — which is not always its
 * midnight, since a spring-forward transition can remove one (see
 * `startOfDayInstant`). `daysLater` shifts by whole calendar days, which is how
 * an EXCLUSIVE range end is spelled from an inclusive "to" day — the next day's
 * start — without an arithmetic step across a DST change.
 */
export function startOfDayInZone(
  dateOnly: string,
  zone: string,
  daysLater = 0,
): string {
  const [year = Number.NaN, month = Number.NaN, day = Number.NaN] = dateOnly
    .split("-")
    .map(Number);
  const shifted = new Date(Date.UTC(year, month - 1, day + daysLater))
    .toISOString()
    .slice(0, 10);
  return new Date(startOfDayInstant(shifted, zone)).toISOString();
}

/** The day's last instant, 23:59:59.999 on `zone`'s wall clock. */
export function endOfDayInZone(dateOnly: string, zone: string): string {
  return new Date(
    instantInZone(dateOnly, zone, 23, 59, 59) + 999,
  ).toISOString();
}

/** City-first labels keep typeahead useful; values remain IANA identifiers. */
export function timezoneOptions(locale: string, selected: string, at: Date) {
  const zones = new Set([
    UTC_ZONE,
    browserZone(),
    ...(typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("timeZone")
      : []),
    ...(selected ? [selected] : []),
  ]);
  const collator = new Intl.Collator(locale);
  return Array.from(zones, (zone) => timezoneOption(locale, zone, at)).sort(
    (left, right) => collator.compare(left.label, right.label),
  );
}

export function timezoneOption(locale: string, zone: string, at: Date) {
  return { value: zone, label: timezoneOptionLabel(locale, zone, at) };
}

function timezoneOptionLabel(locale: string, zone: string, at: Date): string {
  if (zone === UTC_ZONE) return UTC_ZONE;
  const parts = zone.replaceAll("_", " ").split("/");
  const city = parts.pop() ?? zone;
  const place = parts.length ? `${city} (${parts.join(" / ")})` : city;
  try {
    const offset = new Intl.DateTimeFormat(locale, {
      timeZone: zone,
      timeZoneName: "shortOffset",
    })
      .formatToParts(at)
      .find((part) => part.type === "timeZoneName")?.value;
    return offset ? `${place} · ${offset.replace("GMT", "UTC")}` : place;
  } catch (error) {
    if (!(error instanceof RangeError)) throw error;
    // Preserve a saved zone when the browser's timezone database is older.
    return place;
  }
}
