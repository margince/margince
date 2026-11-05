import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { throwProblem } from "../../lib/problem";

type WorkingHours = components["schemas"]["WorkingHours"];

// ISO-8601 weekday numbers, Monday first: the order a week is read in, and the
// order the server stores.
export const WEEK = [1, 2, 3, 4, 5, 6, 7] as const;

// dayLabelKey names each day's own key, spelled out rather than composed.
// `t` takes a key from a closed union, so a computed key does not compile. That
// keeps this list to the days it has words for.
export function dayLabelKey(day: (typeof WEEK)[number]): MessageKey {
  switch (day) {
    case 1:
      return "workingHours.day.1";
    case 2:
      return "workingHours.day.2";
    case 3:
      return "workingHours.day.3";
    case 4:
      return "workingHours.day.4";
    case 5:
      return "workingHours.day.5";
    case 6:
      return "workingHours.day.6";
    default:
      return "workingHours.day.7";
  }
}

export function useWorkingHours(refreshOnReturn = false) {
  return useQuery({
    queryKey: ["working-hours"],
    refetchOnWindowFocus: refreshOnReturn ? "always" : false,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/me/working-hours");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// minutesOf reads `HH:MM` as minutes past midnight, so two times can be
// compared without a date to attach them to. NaN for anything else, which the
// caller treats as "cannot compare" rather than as zero.
export function minutesOf(written: string): number {
  const [hour, minute] = written.split(":");
  const hours = Number(hour);
  const minutes = Number(minute);
  if (!Number.isInteger(hours) || !Number.isInteger(minutes)) {
    return Number.NaN;
  }
  return hours * 60 + minutes;
}

// narrowing reports whether the saved window is smaller than the one before.
// That means fewer hours in a day, or fewer days in the week.
//
// The card owes the reader a sentence here. A host who moves to 09:00-13:00
// will receive roughly half the bookings they do today and may not connect the
// two. Saying so at the moment they save is the difference between a setting
// and a trap.
export function narrowing(before: WorkingHours, after: WorkingHours): boolean {
  const wasHours = minutesOf(before.end_time) - minutesOf(before.start_time);
  const nowHours = minutesOf(after.end_time) - minutesOf(after.start_time);
  if (Number.isNaN(wasHours) || Number.isNaN(nowHours)) {
    return false;
  }
  return nowHours < wasHours || after.days.length < before.days.length;
}
