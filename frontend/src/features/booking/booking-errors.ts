import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import { problemFieldErrors, throwProblem } from "../../lib/problem";

export function bookingWindowCode(problem: unknown) {
  return problemFieldErrors(problem).find(
    (field) =>
      field.code === "booking_horizon" ||
      field.code === "booking_notice" ||
      field.code === "booking_limits",
  )?.code;
}

// The page no longer offers the length the guest chose; its profile names the lengths it does.
export function lengthNotOffered(problem: unknown) {
  return problemFieldErrors(problem).some(
    (field) => field.code === "length_not_offered",
  );
}

export function throwBookingProblem(
  problem: components["schemas"]["Problem"],
  t: (key: MessageKey) => string,
): never {
  return throwProblem(problem, t);
}
