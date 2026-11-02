// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
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

// One hour from now is enough: every selected calendar is read for any window.
const CHECK_WINDOW_MS = 60 * 60 * 1000;

/** Reads the host's own availability once, to learn which calendar fails. */
export function useUnreadableCalendar(enabled: boolean) {
  const check = useQuery({
    queryKey: ["reliable-availability", "calendar-check"],
    enabled,
    retry: false,
    queryFn: async () => {
      const from = new Date();
      const { data, error } = await api.GET("/availability", {
        params: {
          query: {
            from: from.toISOString(),
            to: new Date(from.getTime() + CHECK_WINDOW_MS).toISOString(),
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
  const name =
    calendars.data?.find((item) => item.id === calendar)?.name ?? calendar;
  return <ErrorLine>{t("scheduling.calendarUnreadable", { name })}</ErrorLine>;
}
