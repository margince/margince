// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { UTC_ZONE } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import {
  StandingTimeline,
  type TimelineEvent,
  type TimelineMark,
} from "./standingtimeline";

const meta: Meta<typeof StandingTimeline> = {
  title: "Components/Text and data display/Standing timeline",
  component: StandingTimeline,
  args: { record: UTC_ZONE },
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "52rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof StandingTimeline>;

const company: TimelineEvent[] = [
  {
    key: "k",
    at: "2026-06-03",
    title: "Kickoff call",
    sub: "3 Jun · meeting",
    kind: "done",
  },
  {
    key: "p",
    at: "2026-06-18",
    title: "Proposal sent",
    sub: "18 Jun · Fleet retrofit",
    kind: "done",
  },
  {
    key: "s",
    at: "2026-07-20",
    title: "Site visit",
    sub: "20 Jul · notes",
    kind: "done",
  },
  {
    key: "r",
    at: "2026-07-25",
    title: "Renewal paperwork",
    sub: "due 25 Jul · late",
    kind: "late",
  },
  {
    key: "d",
    at: "2026-08-01",
    title: "Dana wrote",
    sub: "1 Aug · last contact",
    kind: "last",
  },
  {
    key: "w",
    at: "2026-08-20",
    title: "Walkthrough",
    sub: "20 Aug · booked",
    kind: "ahead",
  },
  {
    key: "c",
    at: "2026-09-30",
    title: "Expected close",
    sub: "30 Sep · €48,000",
    kind: "end",
  },
];

/** Four months of an account: month lines, twelve quiet days, today. */
export const Months: Story = {
  args: {
    label: "Brandt Automotive over time",
    asOf: "2026-08-13T09:00:00Z",
    events: company,
    gap: { from: "2026-08-01", title: "12 days", sub: "no reply" },
  },
  play: async ({ canvasElement }) => {
    const list = within(canvasElement).getByRole("list", {
      name: "Brandt Automotive over time",
    });
    await expect(within(list).getAllByRole("listitem")).toHaveLength(7);
  },
};

/** A lead a fortnight old: the axis counts weeks. */
export const Weeks: Story = {
  args: {
    label: "Jonas Weber over time",
    asOf: "2026-08-13T09:00:00Z",
    gap: { from: "2026-08-09", title: "4 days", sub: "your move" },
    events: [
      {
        key: "f",
        at: "2026-08-02",
        title: "Web form",
        sub: "2 Aug · fleet tracking",
        kind: "done",
      },
      {
        key: "r",
        at: "2026-08-04",
        title: "We replied",
        sub: "4 Aug · in 2 h",
        kind: "done",
      },
      {
        key: "j",
        at: "2026-08-09",
        title: "Jonas replied",
        sub: "9 Aug · last contact",
        kind: "last",
      },
      {
        key: "d",
        at: "2026-08-15",
        title: "Demo",
        sub: "15 Aug · booked",
        kind: "ahead",
      },
    ],
  },
};

/** Nine months of a project, with its target end at the axis's last day. */
export const LongProject: Story = {
  args: {
    label: "Depot rollout over time",
    asOf: "2026-08-13T09:00:00Z",
    gap: { from: "2026-08-06", title: "7 days", sub: "no update" },
    events: [
      {
        key: "o",
        at: "2026-03-01",
        title: "Initiative opened",
        sub: "1 Mar · by Lena",
        kind: "done",
      },
      {
        key: "c",
        at: "2026-04-01",
        title: "Contract signed",
        sub: "1 Apr · now delivering",
        kind: "done",
      },
      {
        key: "w",
        at: "2026-06-02",
        title: "Phase one won",
        sub: "2 Jun · €4,500",
        kind: "done",
      },
      {
        key: "a",
        at: "2026-07-03",
        title: "Kickoff agenda",
        sub: "due 3 Jul · late",
        kind: "late",
      },
      {
        key: "s",
        at: "2026-08-06",
        title: "Site report",
        sub: "6 Aug · last update",
        kind: "last",
      },
      {
        key: "e",
        at: "2026-11-30",
        title: "Target end",
        sub: "30 Nov",
        kind: "end",
      },
    ],
  },
};

/** Five events in one week: the canvas opens rows under the three rather
 *  than print one label on another. */
export const Crowded: Story = {
  args: {
    label: "A busy week over time",
    asOf: "2026-08-13T09:00:00Z",
    events: [
      "Retrofit proposal",
      "Expansion workshop",
      "Following up on capacity",
      "Revised offer",
      "ROI discussion",
    ].map(
      (title, index): TimelineEvent => ({
        key: `busy-${index}`,
        at: `2026-08-0${index + 3}`,
        title,
        sub: `${index + 3} Aug · email`,
        kind: index === 4 ? "last" : "done",
      }),
    ),
  },
  play: async ({ canvasElement }) => {
    const labels = canvasElement.querySelectorAll(".standing-timeline-label");
    const tops = new Set(
      [...labels].map((label) => label.getBoundingClientRect().top),
    );
    await expect(tops.size).toBe(labels.length);
  },
};

/** A record with nothing dated: one quiet sentence, no empty axis. */
export const NothingDated: Story = {
  args: {
    label: "A new record over time",
    asOf: "2026-08-13T09:00:00Z",
    events: [],
  },
};

const mark = (
  key: string,
  at: string,
  when: string,
  kind: TimelineMark["kind"],
  title: string,
  direction?: TimelineMark["direction"],
): TimelineMark => ({
  key,
  at,
  precision: "instant",
  kind,
  direction,
  what: kind === "email" ? "Email" : kind === "call" ? "Call" : "Note",
  title,
  detail: direction === "inbound" ? "From Dana Reyes" : undefined,
  when,
});

/** Logged mail and calls as chips above the line. Two a day apart share one
 *  chip with a count, and the ones near Today are held back from its pill. */
export const WithMarks: Story = {
  args: {
    label: "Brandt Automotive over time",
    asOf: "2026-08-13T09:00:00Z",
    events: company,
    marks: [
      mark("1", "2026-06-10", "10 Jun", "email", "Scope question", "inbound"),
      mark("2", "2026-06-24", "24 Jun", "call", "Pricing call", "outbound"),
      mark("3", "2026-07-08", "8 Jul", "note", "Site visit notes"),
      mark("4", "2026-07-09", "9 Jul", "email", "Revised offer", "outbound"),
      mark("5", "2026-08-11", "11 Aug", "email", "Re: walkthrough", "inbound"),
      mark("6", "2026-08-12", "12 Aug", "email", "Re: walkthrough", "outbound"),
    ],
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getAllByRole("button").length,
    ).toBeGreaterThan(2);
  },
};

/** An expected close years out sits past a break that names its month and
 *  year, so the weeks of recent mail keep their room on the line. */
export const FarClose: Story = {
  args: {
    label: "Routine over time",
    asOf: "2026-10-30T09:00:00Z",
    gap: { from: "2026-09-23", title: "37 days", sub: "waiting on them" },
    events: [
      {
        key: "w",
        at: "2026-09-09",
        title: "4 exchanges",
        sub: "7 Sep–9 Sep",
        kind: "done",
      },
      {
        key: "f",
        at: "2026-09-21",
        title: "2 exchanges",
        sub: "14 Sep–21 Sep",
        kind: "done",
      },
      {
        key: "l",
        at: "2026-09-23",
        title: "Re: Demo for Routine",
        sub: "23 Sep · Last contact",
        kind: "last",
      },
      {
        key: "c",
        at: "2029-08-29",
        title: "Expected close",
        sub: "29 Aug · €4,320",
        kind: "end",
      },
    ],
    marks: [
      mark("1", "2026-09-07", "7 Sep", "email", "Demo request", "outbound"),
      mark("2", "2026-09-08", "8 Sep", "email", "Re: demo", "outbound"),
    ],
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("August 2029"),
    ).toBeInTheDocument();
  },
};
