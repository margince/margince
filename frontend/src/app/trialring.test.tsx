/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import type { TrialStanding } from "../lib/license-posture";
import { TrialRing } from "./trialring";

function standing(daysLeft: number): TrialStanding {
  return {
    endsAt: "2026-11-16T09:30:00Z",
    totalDays: 14,
    daysLeft,
    ended: false,
    closing: daysLeft <= 7,
  };
}

function ring(daysLeft: number) {
  const { container } = render(
    <LocaleProvider initial="en">
      <TrialRing standing={standing(daysLeft)} />
    </LocaleProvider>,
  );
  return container;
}

afterEach(cleanup);

it("draws one mark a day, the days still to come lit", () => {
  const marks = ring(10).querySelectorAll(".artrialring-marks path");
  expect(marks).toHaveLength(14);
  expect(
    [...marks].filter((mark) => mark.classList.contains("left")),
  ).toHaveLength(10);
});

it("counts the days on the orb only in the last week", () => {
  expect(ring(10).querySelector(".artrialring-count")).toBeNull();
  cleanup();
  const closing = ring(6);
  expect(closing.querySelector(".artrialring-count")?.textContent).toBe("6");
  expect(
    closing.querySelector(".artrialring")?.hasAttribute("data-closing"),
  ).toBe(true);
});

it("leaves one lit mark on the last day, just before twelve o'clock", () => {
  const marks = [...ring(1).querySelectorAll(".artrialring-marks path")];
  expect(marks.map((mark) => mark.classList.contains("left"))).toEqual([
    ...Array.from({ length: 13 }, () => false),
    true,
  ]);
});
