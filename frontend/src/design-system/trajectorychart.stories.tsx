// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { TrajectoryChart, type TrajectoryChartProps } from "./trajectorychart";

const meta: Meta = {
  title: "Components/Text and data display/Trajectory chart",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 860 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

const amount = (minor: number) => `€${Math.round(minor / 100_000)}k`;
const weeks: [string, number, number][] = [
  ["4 Oct", 0.04, 6_500_000],
  ["11 Oct", 0.12, 14_000_000],
  ["18 Oct", 0.2, 23_500_000],
  ["25 Oct", 0.27, 33_000_000],
  ["31 Oct", 0.33, 42_000_000],
];

// A quarter a third in, short of its target on what is committed.
const quarter: TrajectoryChartProps = {
  label: "Sales won through the period",
  dataLabel: "View chart as table",
  valueLabel: "Actual",
  points: weeks.map(([label, at, value]) => ({
    key: label,
    label,
    at,
    value,
    amount: amount(value),
  })),
  nowLabel: "Today · 31 Oct",
  ticks: [
    { at: 0, label: "October" },
    { at: 0.337, label: "November" },
    { at: 0.663, label: "December" },
  ],
  axisLabel: amount,
  target: { value: 80_000_000, label: "Target €800k" },
  projection: { value: 76_000_000, label: "€760k projected" },
  range: { low: 61_000_000, high: 98_000_000, label: "Modelled finish" },
  shortfall: "€40k short of target",
  onSelect: () => undefined,
};

export const QuarterShortOfTarget: Story = {
  render: () => <TrajectoryChart {...quarter} />,
  play: async ({ canvasElement }) => {
    await expect(
      canvasElement.querySelector(".trajectory-chart-gap"),
    ).not.toBeNull();
  },
};

export const QuarterShortOfTargetDark: Story = {
  ...QuarterShortOfTarget,
  globals: { theme: "dark" },
};

// At a phone's width the range keeps its band but drops its words.
export const QuarterOnAPhone: Story = {
  ...QuarterShortOfTarget,
  globals: { viewport: { value: "phone" } },
};

// A closed period: the line runs to the end, with nothing projected.
export const PeriodClosed: Story = {
  render: () => (
    <TrajectoryChart
      {...quarter}
      points={quarter.points.map((point) => ({
        ...point,
        at: point.at / 0.33,
      }))}
      nowLabel="Through 31 Dec"
      projection={undefined}
      range={undefined}
      shortfall={undefined}
    />
  ),
};
