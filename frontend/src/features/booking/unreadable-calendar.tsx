// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { ProblemError, unwrap } from "../../lib/problem";
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

/**
 * Asks the server once which saved calendar it could not read. The check skips
 * the booking window, so a notice period never hides the answer. A save, a
 * reconnect or coming back to the tab asks again.
 */
export function useUnreadableCalendar(saved: Profile, connection: string) {
  const check = useQuery({
    queryKey: [
      "scheduling-calendar-health",
      saved.provider,
      saved.calendar_id,
      saved.blocking_calendars ?? [],
      connection,
    ],
    enabled: saved.provider !== "" && connection !== "",
    retry: false,
    refetchOnWindowFocus: "always",
    queryFn: async () => unwrap(await api.GET("/scheduling/calendar-health")),
  });
  return check.data?.unreadable_calendar ?? null;
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
