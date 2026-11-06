import type { components } from "../api/schema";
import { isoDay } from "../format/calendarday";

type Employment = components["schemas"]["Contact360Employment"];

// This mirrors employment.IsCurrentSQL so the contact page and deal
// picker agree about which jobs are current.

// Employment dates use the reader's calendar day, matching dates the editor sends.
// UTC can name another day near midnight.
export function today(): string {
  return isoDay(new Date());
}

// A future departure keeps the job current until its last day.
export function stillHeld(employment: Employment): boolean {
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
    return end.slice(0, 7) >= today().slice(0, 7);
  }
  return end > today();
}

// An expired primary flag must not make a former employer look current.
export function currentEmployer(
  employments: ReadonlyArray<Employment> | undefined,
): Employment | undefined {
  return employments?.find(
    (employment) => employment.is_current_primary && stillHeld(employment),
  );
}

// Unknown dates/status and additional current jobs are not former employers.
export function formerEmployers(
  employments: ReadonlyArray<Employment> | undefined,
): ReadonlyArray<Employment> {
  return (employments ?? []).filter(
    (employment) =>
      !stillHeld(employment) && employment.employment_status !== "unknown",
  );
}
