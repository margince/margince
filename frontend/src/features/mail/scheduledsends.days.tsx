// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { SectionHeader } from "../../design-system/atoms";
import { formatDayLong } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { ScheduledSend } from "./scheduledqueue";
import { type Day, relativeDay } from "./scheduledtimeline";

/**
 * The waiting queue as a timeline: one block per calendar day, each headed by
 * the day it names. Today and tomorrow are said in words, because "Tuesday 20
 * October" asks the reader to work out which of the two they are looking at.
 */
export function TimelineDays({
  days,
  now,
  zone,
  renderRow,
}: Readonly<{
  days: readonly Day[];
  now: Date;
  zone: string;
  renderRow: (send: ScheduledSend) => ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <ol className="scheduledsends-days">
      {days.map(({ day, sends }) => {
        const relative = relativeDay(day, now, zone);
        const date = formatDayLong(sends[0]?.scheduled_at ?? "", locale, zone);
        return (
          <li key={day} className="scheduledsends-day">
            <SectionHeader
              level={3}
              title={
                relative ? `${t(`sched.day.${relative}`)} · ${date}` : date
              }
            />
            <ol className="scheduledsends-list">{sends.map(renderRow)}</ol>
          </li>
        );
      })}
    </ol>
  );
}
