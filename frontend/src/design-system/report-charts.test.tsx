/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BulletChart, CumulativeChart, RangeChart } from "./report-charts";

afterEach(cleanup);
const labels = {
  label: "Bookings",
  dataLabel: "Show readings",
  valueLabel: "Actual",
};
it("keeps gaps in the curve and opens evidence for the selected observation", () => {
  const select = vi.fn();
  const { container } = render(
    <CumulativeChart
      {...labels}
      comparisonLabel="Previous"
      axisLabel={String}
      onSelect={select}
      readings={[
        { key: "a", label: "Sep 1", value: 10, amount: "€10" },
        { key: "b", label: "Sep 2", value: null, amount: "Unavailable" },
        { key: "c", label: "Sep 3", value: 30, amount: "€30" },
      ]}
    />,
  );
  expect(
    container.querySelectorAll("polyline.report-chart-actual"),
  ).toHaveLength(2);
  expect(container.querySelectorAll(".report-chart-point")).toHaveLength(2);
  fireEvent.click(screen.getByRole("button", { name: "Sep 3: €30" }));
  expect(select).toHaveBeenCalledWith("c");
  expect(screen.getByText("Unavailable")).toBeTruthy();
});
it("uses a common scale and keeps a zero target distinct from an absent target", () => {
  const { container } = render(
    <BulletChart
      {...labels}
      targetLabel="Target"
      readings={[
        {
          key: "a",
          label: "Maya",
          value: 100,
          amount: "€100",
          target: 0,
          targetAmount: "€0",
        },
        { key: "b", label: "Sam", value: 50, amount: "€50" },
      ]}
    />,
  );
  const fills = [
    ...container.querySelectorAll<HTMLElement>(".report-chart-row"),
  ];
  expect(
    Number.parseFloat(fills[0].style.getPropertyValue("--report-mark-width")),
  ).toBeCloseTo(
    2 *
      Number.parseFloat(fills[1].style.getPropertyValue("--report-mark-width")),
  );
  expect(
    container.querySelectorAll(".report-chart-bullet-target"),
  ).toHaveLength(1);
  expect(container.innerHTML).not.toMatch(/NaN|Infinity/);
  expect(
    screen.getByRole("button", { name: "Maya: €100; Target: €0" }),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Sam: €50" })).toBeTruthy();
});
it("does not draw suppressed stage-age observations", () => {
  const { container } = render(
    <RangeChart
      {...labels}
      upperLabel="75th percentile"
      readings={[
        {
          key: "a",
          label: "Discovery",
          value: null,
          amount: "Insufficient cohort",
          upper: null,
        },
      ]}
    />,
  );
  expect(container.querySelectorAll(".report-chart-range-dot")).toHaveLength(0);
  expect(screen.getAllByText("Insufficient cohort").length).toBeGreaterThan(0);
});
