// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { ProblemError } from "../../lib/problem";
import { problemCode, problemDetails } from "../../lib/problembody";
import { useBookingCalendar } from "./booking-calendar-state";

/**
 * The selected calendar a host's own availability read could not reach. The
 * server names it only to the host, so a public page never has one to show.
 */
export function unreadableCalendarOf(error: unknown): string | null {
  if (
    !(error instanceof ProblemError) ||
    problemCode(error.problem) !== "calendar_unavailable"
  )
    return null;
  const calendar = problemDetails(error.problem)?.calendar;
  return typeof calendar === "string" && calendar !== "" ? calendar : null;
}

type Profile = components["schemas"]["SchedulingProfile"];

// A day past the notice: the read starts where bookings may start, so the
// notice never refuses it before the calendars are read.
const DAY_MS = 24 * 60 * 60 * 1000;

/**
 * Reads the host's own availability once, to learn which saved calendar the
 * server could not read. A save, a reconnect or coming back to the tab asks
 * again.
 */
export function useUnreadableCalendar(saved: Profile, connection: string) {
  const check = useQuery({
    queryKey: [
      "scheduling-calendar-health",
      saved.provider,
      saved.calendar_id,
      saved.blocking_calendars ?? [],
      saved.notice_minutes,
      connection,
    ],
    enabled: saved.provider !== "" && connection !== "",
    retry: false,
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      const from = Date.now();
      const to = from + saved.notice_minutes * 60 * 1000 + DAY_MS;
      const { data, error } = await api.GET("/availability", {
        params: {
          query: {
            from: new Date(from).toISOString(),
            to: new Date(to).toISOString(),
            reliable: true,
          },
        },
      });
      if (error) throw new ProblemError(error);
      return data;
    },
  });
  return unreadableCalendarOf(check.error);
}

/** Names the calendar that could not be read, by the name its provider lists. */
export function UnreadableCalendarLine({
  calendar,
  provider,
}: Readonly<{ calendar: string | null; provider: string }>) {
  const t = useT();
  const { calendars } = useBookingCalendar(provider, calendar !== null);
  if (calendar === null) return null;
  // A calendar the list no longer has is named by nothing: its id is no name.
  const name = calendars.data?.find((item) => item.id === calendar)?.name;
  return (
    <ErrorLine>
      {name
        ? t("scheduling.calendarUnreadable", { name })
        : t("scheduling.savedCalendarUnreadable")}
    </ErrorLine>
  );
}
