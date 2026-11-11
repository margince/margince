// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a calendar mail says happened to its meeting, in words. That is its
// badge, the one line a card has room for, and how its times read. The box
// and the Home card both read these.

import type { components } from "../api/schema";
import {
  activityDay,
  activityTimeSpan,
  type OccurredPrecision,
} from "../format/activitywhen";
import { formatDayMonthNearYear } from "../format/format";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

export type CalendarChange = components["schemas"]["EmailCalendarChange"];
type Guest = components["schemas"]["EmailCalendarGuest"];
export type CalendarBefore = components["schemas"]["EmailCalendarBefore"];

type Tone = "info" | "warning" | "success" | "danger";

/** The badge a change wears: what happened to the meeting, in one word or two. */
export function changeBadge(change: CalendarChange): {
  key: MessageKey;
  tone: Tone;
} {
  switch (change.kind) {
    case "canceled":
      return { key: "meetingChange.badge.canceled", tone: "warning" };
    case "proposed":
      return { key: "meetingChange.badge.proposed", tone: "info" };
    case "answered":
      if (change.answer === "accepted")
        return { key: "meetingChange.badge.accepted", tone: "success" };
      if (change.answer === "declined")
        return { key: "meetingChange.badge.declined", tone: "danger" };
      return { key: "meetingChange.badge.tentative", tone: "info" };
    default:
      return { key: "meetingChange.badge.updated", tone: "info" };
  }
}

/** A guest by the name the calendar gave, else their address. */
export function guestLabel(guest: Guest): string {
  return guest.name?.trim() || guest.email;
}

type When = Readonly<{
  starts_at?: string;
  ends_at?: string;
  starts_precision?: OccurredPrecision;
}>;

/**
 * "2 Dec, 10:00–11:00", or the day alone for an all-day meeting. The year is
 * written only outside now's year.
 */
export function meetingWhen(
  when: When,
  locale: Locale,
  zone: string,
  now: Date = new Date(),
): string | null {
  if (!when.starts_at) return null;
  const precision = when.starts_precision ?? "instant";
  const day = activityDay(
    when.starts_at,
    precision,
    locale,
    zone,
    (at, lang, read) => formatDayMonthNearYear(at, lang, read, now),
  );
  const span = activityTimeSpan(
    when.starts_at,
    when.ends_at,
    precision,
    locale,
    zone,
  );
  return [day, span].filter(Boolean).join(", ");
}

/**
 * The one sentence a change comes to, for a surface with room for a line and
 * not the whole box: a card's subtitle on Home. Each names this meeting's own
 * times, places and people.
 */
export function changeSentence(
  change: CalendarChange,
  t: Translator,
  locale: Locale,
  zone: string,
): string | null {
  const now = meetingWhen(change, locale, zone);
  const by = change.by ? guestLabel(change.by) : null;
  switch (change.kind) {
    case "canceled":
      return now ? t("meetingChange.line.canceled", { when: now }) : null;
    case "proposed":
      return by && now
        ? t("meetingChange.line.proposed", { name: by, when: now })
        : null;
    case "answered":
      return by ? t(answeredKey(change), { name: by }) : null;
  }
  return updateSentence(change, t, locale, zone);
}

// The first change an update names, in the order a reader acts on them.
function updateSentence(
  change: CalendarChange,
  t: Translator,
  locale: Locale,
  zone: string,
): string {
  const now = meetingWhen(change, locale, zone);
  if (change.changed.includes("time") && now) {
    const was = change.before ? meetingWhen(change.before, locale, zone) : null;
    return was
      ? t("meetingChange.line.moved", { was, when: now })
      : t("meetingChange.line.movedTo", { when: now });
  }
  if (change.changed.includes("location"))
    return change.location
      ? t("meetingChange.line.placed", { place: change.location })
      : t("meetingChange.line.unplaced");
  if (change.changed.includes("title") && change.title)
    return t("meetingChange.line.renamed", { title: change.title });
  return guestLines(change, t)[0] ?? t("meetingChange.line.details");
}

/** Who was added and who removed, a line each; empty when neither. */
export function guestLines(change: CalendarChange, t: Translator): string[] {
  const added = change.guests_added ?? [];
  const removed = change.guests_removed ?? [];
  return [
    added.length > 0 &&
      t("meetingChange.line.added", {
        names: added.map(guestLabel).join(", "),
      }),
    removed.length > 0 &&
      t("meetingChange.line.removed", {
        names: removed.map(guestLabel).join(", "),
      }),
  ].filter((line): line is string => Boolean(line));
}

function answeredKey(change: CalendarChange): MessageKey {
  if (change.answer === "accepted") return "meetingChange.line.accepted";
  if (change.answer === "declined") return "meetingChange.line.declined";
  return "meetingChange.line.tentative";
}
