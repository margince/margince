import type { components } from "../api/schema";

type Summary = components["schemas"]["WeeklyNumericSummary"];
export function weeklyNumericStatus(summary: Summary | undefined): {
  basis: "brief.weekly.sharedDefinitions" | "brief.weekly.legacyDefinitions";
  partial: boolean;
  bookingsUnavailable: boolean;
  meetingsUnavailable: boolean;
} {
  const unavailable = (coverage: Summary["bookings_coverage"] | undefined) =>
    coverage !== undefined &&
    (coverage.withheld || !["ok", "no_data"].includes(coverage.status));
  const bookingsUnavailable = unavailable(summary?.bookings_coverage);
  const meetingsUnavailable = unavailable(summary?.meetings_coverage);
  return {
    basis: summary
      ? "brief.weekly.sharedDefinitions"
      : "brief.weekly.legacyDefinitions",
    partial: bookingsUnavailable || meetingsUnavailable,
    bookingsUnavailable,
    meetingsUnavailable,
  };
}
