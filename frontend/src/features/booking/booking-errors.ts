import type { components } from "../../api/schema";
import type { MessageKey } from "../../i18n/en";
import {
  ProblemError,
  problemFieldErrors,
  throwProblem,
} from "../../lib/problem";

export function bookingWindowCode(problem: unknown) {
  return problemFieldErrors(problem).find(
    (field) =>
      field.code === "booking_horizon" ||
      field.code === "booking_notice" ||
      field.code === "booking_limits",
  )?.code;
}

// The page no longer offers the length the guest chose; its profile names the lengths it does.
// A read hands over the problem, and a refused booking the error that carries it.
export function lengthNotOffered(problem: unknown) {
  const body = problem instanceof ProblemError ? problem.problem : problem;
  return problemFieldErrors(body).some(
    (field) => field.code === "length_not_offered",
  );
}

export function throwBookingProblem(
  problem: components["schemas"]["Problem"],
  t: (key: MessageKey) => string,
): never {
  return throwProblem(problem, t);
}
