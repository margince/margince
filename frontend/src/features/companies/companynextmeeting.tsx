// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { StatCard } from "../../design-system/atoms";
import { omitted } from "../../design-system/surfacestate";
import { activityIsAhead } from "../../format/activitywhen";
import { formatDayMonth } from "../../format/format.dates";
import type { Locale, useT } from "../../i18n";
import { WITHHELD_READING } from "./company360health";

type Company360 = components["schemas"]["Company360"];

/**
 * The meeting booked ahead, or nothing. A meeting dated at or before `as_of`
 * has happened, so it is history on the timeline, not something ahead. The
 * strip cell and the timeline's "ahead" stop both read it here, so the two
 * never name different meetings.
 */
export function upcomingMeeting(
  view: Company360,
  record: string,
): NonNullable<Company360["next_meeting"]> | undefined {
  const meeting = view.next_meeting;
  return meeting &&
    activityIsAhead(
      meeting.starts_at,
      meeting.occurred_precision,
      Date.parse(view.as_of),
      record,
    )
    ? meeting
    : undefined;
}

/** The strip's first cell: the day of the next meeting and its subject. */
export function NextMeetingStat({
  view,
  locale,
  recordZone,
  onOpen,
  t,
}: Readonly<{
  view: Company360;
  locale: Locale;
  recordZone: string;
  onOpen?: () => void;
  t: ReturnType<typeof useT>;
}>) {
  const slot = { label: t("co.strip.nextMeeting") } as const;
  if (omitted(view, "next_meeting")) {
    return <StatCard onOpen={onOpen} {...slot} value={t(WITHHELD_READING)} />;
  }
  const meeting = upcomingMeeting(view, recordZone);
  if (!meeting) {
    return (
      <StatCard onOpen={onOpen} {...slot} value={t("co.strip.next.none")} />
    );
  }
  return (
    <StatCard
      onOpen={onOpen}
      {...slot}
      value={formatDayMonth(meeting.starts_at, locale, recordZone)}
      detail={meeting.subject}
    />
  );
}
