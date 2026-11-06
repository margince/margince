// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { activityDay } from "../../format/activitywhen";
import { formatDayMonthNearYear, formatNumber } from "../../format/format";
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
  asOf,
}: Readonly<{
  next?: NextMeeting;
  // Null while meetings ahead may still sit on a page not read yet.
  booked: number | null;
  held: readonly Activity[];
  heldMore: boolean;
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
  const last = held.find((meeting) => meeting.meeting_status !== "canceled");
  const none = t("contact.meetings.noneYet");
  return (
    <StatStrip density="plate" label={t("contact.meetings.figures")}>
      <StatCard
        label={t("contact.meetings.nextLabel")}
        value={next ? day(next.starts_at, next.occurred_precision) : none}
      />
      <StatCard
        label={t("contact.meetings.bookedLabel")}
        value={booked === null ? "…" : formatNumber(booked, locale)}
      />
      <StatCard
        label={t("contact.meetings.heldLabel")}
        value={`${formatNumber(held.length, locale)}${heldMore ? "+" : ""}`}
      />
      <StatCard
        label={t("contact.meetings.lastHeld")}
        value={last ? day(last.occurred_at, last.occurred_precision) : none}
      />
    </StatStrip>
  );
}
