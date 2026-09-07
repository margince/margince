// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The two zones this product renders dates in, and the rule for which one a
// screen owes its reader. `formatDate`/`formatDateTime`/`formatDateAbbrev` take
// their zone as an argument and only attach it (format.ts, zone-by-purpose) —
// picking the zone is the caller's judgement, and this file is where that
// judgement is written down once.
//
// The two purposes are NOT interchangeable, and each has its own way of being
// wrong:
//
//   useRecordZone() — the organization's own clock, as the installation
//   configured it (`installation.timezone`, served by app/recordzone.tsx). Its
//   dates belong to the record, not to whoever is looking at it. Following the
//   reader here MISSTATES the record: a close date, a renewal, an invoice's
//   issue day and a timeline's day headings have to read the same for every
//   colleague, or two people quoting the same page quote different days. It is
//   also the only correct answer for a date-only wire value (OpenAPI
//   `format: date`): there is no instant in `2026-08-21` to localize, and
//   reading it in a zone behind UTC prints the day before.
//
//   It is a HOOK rather than a constant because the answer belongs to the
//   installation and arrives over the wire. A plain helper that needs it takes
//   it as a parameter from the component that read it — never by importing the
//   fallback below, which is a different value that only happens to agree.
//
//   viewerZone() — the reader's own clock, for a moment they relate to
//   themselves: when a credential they are lending expires, when a paused job
//   resumes, how stale a mailbox is, when a slot they are about to book starts.
//   Pinning these to a fixed zone shows a reader outside it a different
//   calendar day than the one the thing actually happens on, which is how the
//   consent screen came to promise an expiry on a day that had not arrived.
//
// When a site is genuinely both — a personal deadline on a record page — the
// page wins: a record surface is ONE clock, so a due date and the activity
// beside it can never be read in two different zones.
//
// "Genuinely both" means both readings are defensible, and that is a claim about
// the STORED value, not about the screen. An activity's `due_at` is minted by
// `dueInstant` as the end of the picked day in the BROWSER's zone, so the wire
// value already carries the picker's clock; the record zone does not read the
// organization's day out of it, it reads a day the picker never chose, off by
// one for every reader outside that zone. A value with no record reading has
// nothing for the page to win against, so `due_at` takes viewerZone() wherever
// it is shown — the tasks queue, the record's next steps, the task detail —
// while the activity's `occurred_at` beside it stays on the record zone,
// because when something happened IS a fact about the record.

// The zone a record's dates are read in when the installation's own answer is
// not on hand. It is the LAST resort, not the value: `useRecordZone` in
// app/recordzone.tsx serves `installation.timezone`, and the authenticated
// shell holds its first paint until that read lands, so no signed-in surface
// renders a record date against this constant.
//
// It stays a real zone rather than UTC because the surfaces that can still
// reach it are the ones outside the settings read — a story, a test that mounts
// a screen bare — and those are easier to read against a zone with an offset
// and a DST rule than against one with neither.
//
// Do not import it into a screen. The gate in zone-by-purpose.test.ts refuses
// that, and names this comment when it does.
export const FALLBACK_RECORD_ZONE = "Europe/Berlin";

/**
 * The zone the reader's own browser is in, or `UTC` when it will not say.
 *
 * A function, not a constant, on purpose. A module-level constant is evaluated
 * once at import time, which is before the app has rendered anything — so it
 * would freeze the answer for the whole session, outlive a reader who changed
 * their machine's zone or crossed one, and be captured before a test that
 * pretends to be elsewhere had installed its own answer. Constructing an
 * `Intl.DateTimeFormat` per call is cheap next to the render it feeds.
 *
 * A runtime with no zone to report leaves `timeZone` empty, and an empty string
 * is not a zone Intl accepts — it throws. `UTC` is the honest fallback: it
 * names a real zone, and it is the one every wire instant is already stored in.
 */
/**
 * The reader's own zone, named and offset — "Indochina Time · UTC+7".
 *
 * Every line about a moment a message will be RELEASED at carries it. A send
 * scheduled from Ho Chi Minh City for an account in Munich is a moment in ONE
 * of those places, and "Tuesday, 08:00" with no zone is the sentence that gets
 * that wrong.
 *
 * Both halves come from Intl's own names, so the zone reads in the reader's
 * language; the offset is spelled UTC rather than Intl's GMT, because that is
 * what the rest of the product says.
 */
export function zoneNameAndOffset(locale: string, at: Date): string {
  const named = (style: "long" | "shortOffset") =>
    new Intl.DateTimeFormat(locale, { timeZoneName: style })
      .formatToParts(at)
      .find((piece) => piece.type === "timeZoneName")?.value ?? "";
  return [named("long"), named("shortOffset").replace("GMT", "UTC")]
    .filter(Boolean)
    .join(" · ");
}

export function viewerZone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

// The offset (ms) `zone` sits at relative to UTC at the instant `utcMs`
// names — positive east of UTC, negative west. Derived by re-reading the
// instant's wall clock through the zone and comparing it back against UTC,
// the standard technique for converting a zoned wall clock to an instant
// without a timezone-database library.
function zoneOffsetMs(utcMs: number, zone: string): number {
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
  const asIfUtc = Date.UTC(
    Number(parts.year),
    Number(parts.month) - 1,
    Number(parts.day),
    Number(parts.hour),
    Number(parts.minute),
    Number(parts.second),
  );
  return asIfUtc - utcMs;
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
  const [year, month, day] = dateOnly.split("-").map(Number);
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
function dayInZone(utcMs: number, zone: string): string {
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
  const minute = 60_000;
  // Two hours covers every transition tzdata records; the loop exits at the
  // first minute that reads as the wanted day.
  for (let step = 1; step <= 120; step += 1) {
    const candidate = resolved + step * minute;
    if (dayInZone(candidate, zone) === dateOnly) {
      return candidate;
    }
  }
  // No minute in two hours reads as the wanted day. That is not a zone rule
  // this code knows how to interpret, and guessing would silently file records
  // under a day nobody picked — so the honest answer is the unadjusted one,
  // which is what every caller got before this correction existed.
  return resolved;
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
  const [year, month, day] = dateOnly.split("-").map(Number);
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
