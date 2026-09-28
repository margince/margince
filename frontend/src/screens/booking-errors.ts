import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { problemFieldErrors, throwProblem } from "./common";

export function bookingWindowCode(problem: unknown) {
  return problemFieldErrors(problem).find(
    (field) =>
      field.code === "booking_horizon" ||
      field.code === "booking_notice" ||
      field.code === "booking_limits",
  )?.code;
}

export function throwBookingProblem(
  problem: components["schemas"]["Problem"],
  t: (key: MessageKey) => string,
): never {
  switch (bookingWindowCode(problem)) {
    case "booking_horizon":
      return throwProblem({
        ...problem,
        detail: t("scheduling.windowHorizon"),
      });
    case "booking_notice":
      return throwProblem({ ...problem, detail: t("scheduling.windowNotice") });
    case "booking_limits":
      return throwProblem({ ...problem, detail: t("scheduling.windowLimits") });
    default:
      return throwProblem(problem);
  }
}
