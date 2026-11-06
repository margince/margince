// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { activityDay } from "../../format/activitywhen";
import {
  formatDayMonthNearYear,
  formatNumber,
  MONEY_ABSENT,
} from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";

type Activity = components["schemas"]["Activity"];
type NextMeeting = NonNullable<
  components["schemas"]["Contact360"]["next_meeting"]
>;

/**
 * The tab in four figures: the next meeting, how many are booked, how many
 * were held and when the last one was. A count still waiting on a page not
 * read yet says so with a "+" rather than undercounting.
 */
export function MeetingFigures({
  next,
  booked,
  held,
  heldMore,
  withheld = false,
  asOf,
}: Readonly<{
  next?: NextMeeting;
  // Null while meetings ahead may still sit on a page not read yet.
  booked: number | null;
  // Meetings held, newest first, without those called off. Null until read.
  held: readonly Activity[] | null;
  heldMore: boolean;
  // The meetings are not this reader's to see, so the counts stay unknown.
  withheld?: boolean;
  asOf: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const now = new Date(asOf);
  const day = (at: string, precision: Activity["occurred_precision"]) =>
    activityDay(at, precision, locale, zone, (iso, l, z) =>
      formatDayMonthNearYear(iso, l, z, now),
    );
  // A page not read yet may hold the last meeting held.
  const last = held?.[0];
  const none = t("contact.meetings.noneYet");
  const unread = withheld ? MONEY_ABSENT : "…";
  let lastValue = none;
  if (last) lastValue = day(last.occurred_at, last.occurred_precision);
  else if (held === null || heldMore) lastValue = unread;
  const heldValue =
    held === null
      ? unread
      : `${formatNumber(held.length, locale)}${heldMore ? "+" : ""}`;
  return (
    <StatStrip density="plate" label={t("contact.meetings.figures")}>
      <StatCard
        label={t("contact.meetings.nextLabel")}
        value={next ? day(next.starts_at, next.occurred_precision) : none}
      />
      <StatCard
        label={t("contact.meetings.bookedLabel")}
        value={booked === null ? unread : formatNumber(booked, locale)}
      />
      <StatCard label={t("contact.meetings.heldLabel")} value={heldValue} />
      <StatCard label={t("contact.meetings.lastHeld")} value={lastValue} />
    </StatStrip>
  );
}
