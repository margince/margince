import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useLocale, useT } from "../i18n";
import { bookingWindowCode, throwBookingProblem } from "./booking-errors";
import type { useWorkingHours } from "./working-hours";

export function useInviteAvailability(
  from: string,
  duration: number,
  searchAhead: boolean,
  configured: boolean,
  profile: components["schemas"]["SchedulingProfile"] | undefined,
  hours: ReturnType<typeof useWorkingHours>["data"],
) {
  const t = useT();
  const { locale } = useLocale();
  const now = Date.now();
  const earliest = now + (profile?.notice_minutes ?? 120) * 60000;
  const latest = now + (profile?.horizon_days ?? 30) * 86400000;
  const outsideHorizon = new Date(from).getTime() >= latest;
  const windowDays = searchAhead ? (profile?.horizon_days ?? 30) : 7;
  const slots = useQuery({
    queryKey: [
      "reliable-availability",
      from,
      locale,
      duration,
      windowDays,
      profile,
      hours,
    ],
    enabled: configured && !outsideHorizon,
    queryFn: async () => {
      let cursor = new Date(from).getTime();
      const end = Math.min(cursor + windowDays * 86400000, latest);
      while (cursor < end) {
        const until = Math.min(cursor + 31 * 86400000, end);
        const { data, error } = await api.GET("/availability", {
          params: {
            query: {
              from: new Date(cursor).toISOString(),
              to: new Date(until).toISOString(),
              duration_minutes: duration,
              reliable: true,
            },
          },
        });
        if (error) {
          if (searchAhead && bookingWindowCode(error) === "booking_horizon")
            return { slots: [], truncated: false };
          throwBookingProblem(error, t);
        }
        if (data.slots.length || until === end) return data;
        // Overlap by a meeting length so a chunk boundary cannot hide a slot.
        cursor = until - duration * 60000;
      }
      return { slots: [], truncated: false };
    },
  });
  return { earliest, latest, outsideHorizon, slots };
}
