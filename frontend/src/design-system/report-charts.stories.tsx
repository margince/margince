import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { formatMoney } from "../format/format";
import { LocaleProvider } from "../i18n";
import {
  BulletChart,
  type ChartReading,
  CumulativeChart,
  GroupedBars,
  RangeChart,
} from "./report-charts";

const meta: Meta = {
  title: "Components/Text and data display/Report charts",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: 780 }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;
const money = (value: number) => formatMoney(value * 100, "EUR", "en");
const trend: readonly ChartReading[] = [
  {
    key: "1",
    label: "Sep 1",
    value: 0,
    amount: "€0",
    comparison: 0,
    comparisonAmount: "€0",
  },
  {
    key: "8",
    label: "Sep 8",
    value: 64000,
    amount: "€64,000",
    comparison: 52000,
    comparisonAmount: "€52,000",
  },
  {
    key: "15",
    label: "Sep 15",
    value: 142000,
    amount: "€142,000",
    comparison: 101000,
    comparisonAmount: "€101,000",
  },
  {
    key: "22",
    label: "Sep 22",
    value: 216000,
    amount: "€216,000",
    comparison: 172800,
    comparisonAmount: "€172,800",
  },
];
function Readings({
  rows = trend,
}: Readonly<{ rows?: readonly ChartReading[] }>) {
  const [selected, select] = useState("");
  return (
    <>
      <CumulativeChart
        readings={rows}
        label="Bookings over September"
        valueLabel="Booked"
        comparisonLabel="Previous month"
        dataLabel="Show readings"
        axisLabel={money}
        reference={{
          value: 300000,
          amount: "€300,000",
          label: "Monthly target",
        }}
        onSelect={select}
      />
      <p aria-live="polite">{selected && `Evidence for ${selected}`}</p>
    </>
  );
}
export const Cumulative: Story = { render: () => <Readings /> };
export const MissingObservations: Story = {
  render: () => (
    <Readings
      rows={trend.map((point, index) =>
        index === 1 ? { ...point, value: null, amount: "Unavailable" } : point,
      )}
    />
  ),
};
export const OneObservation: Story = {
  render: () => <Readings rows={[trend[2]]} />,
};
export const NoObservations: Story = { render: () => <Readings rows={[]} /> };
export const OwnerTargets: Story = {
  render: () => (
    <BulletChart
      label="Monthly bookings by owner"
      dataLabel="Show readings"
      valueLabel="Bookings"
      targetLabel="Target"
      readings={[
        {
          key: "a",
          label: "Maya · DACH enterprise sales",
          value: 120000,
          amount: "€120,000",
          target: 100000,
          targetAmount: "€100,000",
        },
        {
          key: "b",
          label: "Sam",
          value: 24000,
          amount: "€24,000",
          target: 0,
          targetAmount: "€0",
        },
        {
          key: "c",
          label: "Alex",
          value: 48000,
          amount: "€48,000",
          targetAmount: "Target not set",
        },
      ]}
    />
  ),
};
export const StageDistribution: Story = {
  render: () => (
    <RangeChart
      label="Current stage age"
      valueLabel="Median days"
      upperLabel="75th percentile"
      dataLabel="Show readings"
      readings={[
        {
          key: "a",
          label: "Discovery",
          value: 8,
          amount: "8 days",
          upper: 13,
          upperAmount: "13 days",
        },
        {
          key: "b",
          label: "Commercial negotiation",
          value: 23,
          amount: "23 days",
          upper: 37,
          upperAmount: "37 days",
        },
        {
          key: "c",
          label: "Proposal",
          value: null,
          amount: "Insufficient cohort",
          upper: null,
        },
      ]}
    />
  ),
};
export const IndependentOutcomes: Story = {
  render: () => (
    <GroupedBars
      label="Independent weekly outcomes"
      valueLabel="Held meetings"
      comparisonLabel="Accepted opportunities"
      dataLabel="Show readings"
      readings={[
        {
          key: "1",
          label: "Sep 1–7",
          value: 14,
          amount: "14",
          comparison: 4,
          comparisonAmount: "4",
        },
        {
          key: "2",
          label: "Sep 8–14",
          value: 18,
          amount: "18",
          comparison: 7,
          comparisonAmount: "7",
        },
        {
          key: "3",
          label: "Sep 15–21",
          value: 16,
          amount: "16",
          comparison: 9,
          comparisonAmount: "9",
        },
      ]}
    />
  ),
};

export const NoComparisonHistory: Story = {
  render: () => (
    <CumulativeChart
      readings={trend.map((reading) => ({ ...reading, comparison: undefined }))}
      label="Won deal value"
      dataLabel="View chart as table"
      valueLabel="Actual"
      axisLabel={money}
    />
  ),
};

export const OwnerTargetsDark: Story = {
  ...OwnerTargets,
  globals: { theme: "dark" },
};
