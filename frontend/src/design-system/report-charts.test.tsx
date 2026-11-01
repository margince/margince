/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { BulletChart, CumulativeChart, RangeChart } from "./report-charts";
import { niceAxis } from "./report-charts.model";

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

it("ends the axis on a round figure, stepping in ones, twos, fives", () => {
  expect(niceAxis(453_600, 4)).toEqual({
    top: 600_000,
    ticks: [0, 200_000, 400_000, 600_000],
  });
  expect(niceAxis(453_600, 2)).toEqual({
    top: 500_000,
    ticks: [0, 250_000, 500_000],
  });
  expect(niceAxis(0, 4).top).toBe(1);
});
it("washes under each unbroken run and labels the latest reading", () => {
  const { container } = render(
    <CumulativeChart
      {...labels}
      axisLabel={String}
      readings={[
        { key: "a", label: "Sep 1", value: 10, amount: "€10" },
        { key: "b", label: "Sep 2", value: 20, amount: "€20" },
        { key: "c", label: "Sep 3", value: null, amount: "Unavailable" },
        { key: "d", label: "Sep 4", value: 30, amount: "€30" },
        { key: "e", label: "Sep 5", value: 40, amount: "€40" },
      ]}
    />,
  );
  expect(container.querySelectorAll("path.report-chart-area")).toHaveLength(2);
  expect(container.querySelector(".report-chart-end")?.textContent).toBe("€40");
  expect(
    container.querySelectorAll(".report-chart-point[data-last]"),
  ).toHaveLength(1);
});
it("answers a focused reading with a guide line and its tooltip", () => {
  const { container } = render(
    <CumulativeChart
      {...labels}
      comparisonLabel="Previous"
      axisLabel={String}
      onSelect={vi.fn()}
      readings={[
        {
          key: "a",
          label: "Sep 1",
          value: 10,
          amount: "€10",
          comparison: 5,
          comparisonAmount: "€5",
        },
        { key: "b", label: "Sep 2", value: 20, amount: "€20" },
      ]}
    />,
  );
  expect(container.querySelector(".report-chart-tip")).toBeNull();
  fireEvent.focus(screen.getByRole("button", { name: "Sep 1: €10" }));
  const tip = container.querySelector(".report-chart-tip");
  expect(tip?.textContent).toBe("Sep 1€10Previous: €5");
  expect(container.querySelector(".report-chart-crosshair")).not.toBeNull();
  // The end label steps aside while a reading is in hand.
  expect(container.querySelector(".report-chart-end")).toBeNull();
});
