// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { StatCard } from "./atoms";
import { HowItStands } from "./howitstands";
import { StageLadder } from "./stageladder";
import { StandingDims } from "./standingdims";
import { HealthRing } from "./standingring";
import { StandingTimeline } from "./standingtimeline";
import { StatStrip } from "./statstrip";

const meta: Meta<typeof HowItStands> = {
  title: "Components/Layout and structure/How it stands",
  component: HowItStands,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "56rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof HowItStands>;

const noop = () => undefined;

/** A company: verdict, ring beside its rows, timeline, figures. Rules wrote
 *  it, so the pill names the CRM's records, not an agent. */
export const CompanyWithRing: Story = {
  render: () => (
    <HowItStands
      date="13 Aug"
      verdict={{
        word: "Watch",
        sentence:
          "Dana asked for a week on the retrofit review and has been quiet for 12 days.",
        tone: "warning",
      }}
      hero={
        <HealthRing
          word="Watch"
          tone="warning"
          arcs={[
            { key: "r", label: "Relationship", rating: "good" },
            { key: "c", label: "Commercial" },
            { key: "p", label: "Payment", rating: "strong" },
          ]}
        />
      }
      dims={
        <StandingDims
          dims={[
            {
              key: "r",
              label: "Relationship",
              value: "Good",
              sub: "Last met them 12 days ago",
              tone: "warning",
            },
            {
              key: "c",
              label: "Commercial",
              value: "Not rated",
              sub: "No open deal",
              tone: "neutral",
            },
            {
              key: "p",
              label: "Payment",
              value: "Strong",
              sub: "4 of 4 paid on time",
              tone: "success",
            },
          ]}
        />
      }
      timeline={
        <StandingTimeline
          label="Brandt Automotive over time"
          asOf="2026-08-13T09:00:00Z"
          gap={{ from: "2026-08-01", title: "12 days", sub: "no reply" }}
          events={[
            {
              key: "k",
              at: "2026-06-03",
              title: "Kickoff call",
              sub: "3 Jun · meeting",
              kind: "done",
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
          ]}
        />
      }
      figures={
        <StatStrip density="plate">
          <StatCard
            label="Open deals"
            value="2"
            detail="€60,000"
            onOpen={noop}
          />
          <StatCard
            label="Last two-way"
            value="1 Aug"
            tone="warning"
            detail="12 days ago"
          />
          <StatCard
            label="Revenue · 12 mo"
            value="€64,200"
            detail="Net invoiced"
          />
          <StatCard
            label="Overdue"
            value="€0"
            tone="success"
            detail="1 invoice open"
          />
        </StatStrip>
      }
    />
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("heading", {
        name: "How it stands · 13 Aug",
      }),
    ).toBeVisible();
    await expect(
      within(canvasElement).getByText("From CRM records"),
    ).toBeVisible();
  },
};

/** A deal: the verdict a model wrote, so the pill is indigo and counts its
 *  sources; the ladder runs under the rows. */
export const DealWithLadder: Story = {
  render: () => (
    <HowItStands
      date="13 Aug"
      sources={{ by: "model", count: 4 }}
      verdict={{
        word: "Drifting",
        sentence:
          "Offer v2 has gone unanswered and the deal runs through Dana alone.",
        tone: "warning",
      }}
      heroLayout="below"
      hero={
        <StageLadder
          label="Stage"
          variant="bars"
          tone="warning"
          steps={[
            {
              key: "a",
              label: "Qualification",
              done: true,
              sub: "12 d",
              onPick: noop,
            },
            {
              key: "b",
              label: "Discovery",
              current: true,
              sub: "6 d · now",
              onPick: noop,
            },
            { key: "c", label: "Proposal", onPick: noop },
            { key: "w", label: "Closed Won", terminal: true, onPick: noop },
            { key: "l", label: "Closed Lost", terminal: true, onPick: noop },
          ]}
        />
      }
      dims={
        <StandingDims
          dims={[
            {
              key: "m",
              label: "Momentum",
              value: "Stalled",
              sub: "since 3 Aug",
              tone: "danger",
            },
            {
              key: "c",
              label: "Committee",
              value: "2 named",
              sub: "economic buyer missing",
              tone: "warning",
            },
            {
              key: "d",
              label: "Close",
              value: "30 Sep",
              sub: "48 days left · best case",
              tone: "success",
            },
          ]}
        />
      }
    />
  ),
};

/** A project nobody has assessed: neutral, no wash, and nothing invented. */
export const NotAssessed: Story = {
  render: () => (
    <HowItStands
      date="13 Aug"
      verdict={{ word: "Not assessed", tone: "neutral" }}
      dims={
        <StandingDims
          dims={[
            {
              key: "c",
              label: "Commitments",
              value: "None open",
              tone: "neutral",
            },
            {
              key: "t",
              label: "Time used",
              value: "No end date",
              tone: "neutral",
            },
            { key: "d", label: "Deals", value: "None yet", tone: "neutral" },
          ]}
        />
      }
      timeline={
        <StandingTimeline
          label="Over time"
          asOf="2026-08-13T09:00:00Z"
          events={[]}
        />
      }
    />
  ),
};

/** German in a narrow column: the dimensions drop under the verdict instead of
 *  squeezing it, and long names wrap inside their rows. */
export const NarrowGerman: Story = {
  decorators: [
    (Story) => (
      <LocaleProvider initial="de">
        <div lang="de" style={{ maxWidth: "24rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
  render: () => (
    <HowItStands
      date="13. Aug."
      verdict={{
        word: "Gefährdet",
        sentence: "Die Zahlung ist seit 19 Tagen überfällig.",
        tone: "danger",
      }}
      dims={
        <StandingDims
          dims={[
            {
              key: "r",
              label: "Beziehungsqualität",
              value: "Stark",
              sub: "3 Kontakte hier sind im Austausch",
              tone: "success",
            },
            {
              key: "p",
              label: "Zahlungsverhalten",
              value: "Gefährdet",
              sub: "Rechnung seit 19 Tagen offen",
              tone: "danger",
            },
          ]}
        />
      }
    />
  ),
};
