// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { Calendar, type ISODay } from "./calendar";

// Calendar: one month, presentational, with the selected day and the month on
// show both held by the caller. The cases here are the ones that hierarchy
// creates — a day marked vs. nothing marked yet — plus the grid's own reason
// to always draw six weeks: paging between a five-week month and a six-week
// one must not move the height of the thing the reader is looking at.
const TODAY = new Date(2026, 7, 27);

const meta: Meta<typeof Calendar> = {
  title: "Design System/Calendar",
  component: Calendar,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof Calendar>;

// Live, so paging the month and picking a day can both be judged — a calendar
// that only ever rendered its initial props would hide the one thing it owns:
// staying put on the caller's chosen month rather than snapping back to today.
function Live(props: Readonly<{ initial: ISODay | "" }>) {
  const [month, setMonth] = useState(TODAY);
  const [selected, setSelected] = useState<ISODay | "">(props.initial);
  return (
    <Calendar
      month={month}
      onMonthChange={setMonth}
      selected={selected}
      onSelect={setSelected}
      today={TODAY}
      locale="en"
    />
  );
}

// A day already chosen: August's grid opens on a Saturday, so the leading
// outside days from July are on show at the same time as the marked day.
export const DaySelected: Story = {
  render: () => <Live initial="2026-08-15" />,
};

// Nothing chosen yet — no day carries the marked state, which is what a
// calendar opened before the reader has picked anything must show.
export const NoneSelected: Story = {
  render: () => <Live initial="" />,
};

// February 2026 has four weeks that hold real days and two rows of pure
// overflow either side; the grid still draws six weeks so paging into a longer
// month right after does not move the chevrons or whatever sits below the
// calendar in the dialog it lives in.
export const ShortMonthKeepsSixWeeks: Story = {
  render: () => (
    <Calendar
      month={new Date(2026, 1, 1)}
      onMonthChange={() => {}}
      selected=""
      onSelect={() => {}}
      today={TODAY}
      locale="en"
    />
  ),
};
