import type { components } from "../api/schema";
import { todayIn } from "../format/calendarday";
import { useCompanyZone } from "./companyzone";

type Employment = components["schemas"]["Contact360Employment"];

// This mirrors employment.IsCurrentSQL so the contact page and deal
// picker agree about which jobs are current.

// A departure day is a day in the installation's zone, the zone the server
// judges it in. So the day it is today and the day "end now" writes are read
// there, never from the browser's own clock.
export function useEmploymentZone(): string {
  return useCompanyZone();
}

export function today(zone: string): string {
  return todayIn(zone);
}

// A future departure keeps the job current until its last day. A company's
// relationship row carries the same three fields and is judged the same way.
export function stillHeld(
  employment: Pick<
    Employment,
    "employment_status" | "ended_at" | "ended_precision"
  >,
  zone: string,
): boolean {
  if (
    employment.employment_status === "former" ||
    employment.employment_status === "unknown"
  )
    return false;
  if (!employment.ended_at) {
    return true;
  }
  const end = employment.ended_at.slice(0, 10);
  if (employment.ended_precision === "month") {
    return end.slice(0, 7) >= today(zone).slice(0, 7);
  }
  return end > today(zone);
}

// A career date at the precision it was recorded: a month-only date stays a
// month rather than gaining a first of the month. `formatDay` writes a full day.
export function careerDate(
  value: string,
  precision: Employment["started_precision"],
  formatDay: (day: string) => string,
): string {
  return precision === "month"
    ? value.slice(0, 7)
    : formatDay(value.slice(0, 10));
}

// An expired primary flag must not make a former employer look current.
export function currentEmployer(
  employments: ReadonlyArray<Employment> | undefined,
  zone: string,
): Employment | undefined {
  return employments?.find(
    (employment) =>
      employment.is_current_primary && stillHeld(employment, zone),
  );
}

// Unknown dates/status and additional current jobs are not former employers.
export function formerEmployers(
  employments: ReadonlyArray<Employment> | undefined,
  zone: string,
): ReadonlyArray<Employment> {
  return (employments ?? []).filter(
    (employment) =>
      !stillHeld(employment, zone) &&
      employment.employment_status !== "unknown",
  );
}
