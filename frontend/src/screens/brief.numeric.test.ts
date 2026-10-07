import { expect, it } from "vitest";
import {
  sharedWeeklyNumbers,
  unavailableWeeklyNumbers,
} from "./brief.fixtures";
import { weeklyNumericStatus } from "./brief.numeric";

it("keeps a complete zero-activity week readable", () => {
  const result = weeklyNumericStatus({
    ...sharedWeeklyNumbers,
    won_minor: 0,
    bookings_coverage: { status: "no_data", withheld: false },
    meetings_coverage: { status: "no_data", withheld: false },
  });
  expect(result.partial).toBe(false);
  expect(result.bookingsUnavailable).toBe(false);
  expect(result.meetingsUnavailable).toBe(false);
});
it("withholds incomplete shared numbers and identifies legacy editions", () => {
  expect(weeklyNumericStatus(unavailableWeeklyNumbers)).toMatchObject({
    partial: true,
    bookingsUnavailable: true,
    meetingsUnavailable: true,
  });
  expect(weeklyNumericStatus(undefined).basis).toBe(
    "brief.weekly.legacyDefinitions",
  );
});
