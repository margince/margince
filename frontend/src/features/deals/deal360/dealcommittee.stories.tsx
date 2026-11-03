// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, within } from "storybook/test";
import type { components } from "../../../api/schema";
import { StoryProviders } from "../../../testing/story-utils";
import { CommitteeReading } from "./dealcommittee";

// The buying committee as two named sides. Three states are worth seeing: one
// stakeholder with several colleagues, a gap, and a full committee. Each has a
// phone frame and a wide one. A withheld read, an
// empty one and a seat the reader may not name each say their own sentence.

type DealCoverage = components["schemas"]["DealCoverage"];
type Seat = DealCoverage["stakeholders"][number];

const meta: Meta<typeof CommitteeReading> = {
  title: "Records/Deal 360/Committee",
  component: CommitteeReading,
  parameters: { layout: "padded" },
  args: {
    roleLabel: (role: string) => role.replaceAll("_", " "),
    wroteOn: (at: string) => at.slice(0, 10),
    withheld: false,
    pending: false,
  },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof CommitteeReading>;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const dana: Seat = {
  contact_id: "01a03000-0000-7000-8000-0000000000b1",
  contact_name: "Dana Weiss",
  role: "champion",
  engaged: true,
  last_inbound_at: "2026-10-20T09:14:00Z",
  last_inbound_precision: "instant",
};
const tomas: Seat = {
  contact_id: "01a03000-0000-7000-8000-0000000000b2",
  contact_name: "Tomas Berg",
  role: "economic_buyer",
  engaged: true,
  last_inbound_at: "2026-10-14T12:00:00Z",
  last_inbound_precision: "day",
};
const ines: Seat = {
  contact_id: "01a03000-0000-7000-8000-0000000000b3",
  contact_name: "Ines Kraft",
  role: "",
  engaged: false,
};

const coverage = (over: Partial<DealCoverage> = {}): DealCoverage => ({
  deal_id: DEAL_ID,
  stakeholders: [dana, tomas, ines],
  our_side: [],
  risks: [],
  sections_omitted: [],
  ...over,
});

const SIX_COLLEAGUES = [
  {
    key: "u1",
    name: "Lena Fischer",
    jobs: "Owner, Sales owner",
    carries: true,
  },
  { key: "u2", name: "Tomás Reyes", jobs: "Delivery owner", carries: false },
  { key: "u3", name: "Ana Lima", jobs: "Solution engineer", carries: false },
  { key: "u4", name: "Kai Brandt", jobs: "Legal", carries: false },
  { key: "u5", name: "Mia Chen", jobs: "Finance", carries: false },
  { key: "u6", name: "Noah Weber", jobs: "Executive sponsor", carries: false },
];

// The phone frame is the page at 390px less the card's own inset; the wide
// frame is one column of the deal overview at 1440px.
function PhoneFrame({ children }: Readonly<{ children: ReactNode }>) {
  return <div style={{ padding: "var(--padCard)" }}>{children}</div>;
}
function WideFrame({ children }: Readonly<{ children: ReactNode }>) {
  return <div style={{ inlineSize: 520 }}>{children}</div>;
}
const PHONE = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  decorators: [
    (Story: () => ReactNode) => (
      <PhoneFrame>
        <Story />
      </PhoneFrame>
    ),
  ],
};
const WIDE = {
  tags: ["uat-wide"],
  decorators: [
    (Story: () => ReactNode) => (
      <WideFrame>
        <Story />
      </WideFrame>
    ),
  ],
};

async function sides(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  return {
    ours: within(await canvas.findByRole("region", { name: "Your team" })),
    theirs: within(canvas.getByRole("region", { name: "Their team" })),
    legend: within(canvas.getByRole("list", { name: "What the marks mean" })),
  };
}

const oneStakeholder: Story = {
  args: {
    coverage: coverage({
      stakeholders: [dana],
      risks: [{ kind: "single_threaded_theirs", summary: "one thread" }],
    }),
    team: SIX_COLLEAGUES,
  },
  play: async ({ canvasElement }) => {
    const { ours, theirs, legend } = await sides(canvasElement);
    await expect(ours.getAllByRole("listitem")).toHaveLength(6);
    await expect(ours.getByText("Handles most exchanges")).toBeVisible();
    await expect(theirs.getByText("Dana Weiss")).toBeVisible();
    await expect(
      theirs.getByText("Only one contact. Add a second one."),
    ).toBeVisible();
    await expect(legend.getByText("Your team")).toBeVisible();
  },
};

/** One stakeholder, six colleagues, at phone width. */
export const OneStakeholderSeveralColleaguesPhone: Story = {
  ...oneStakeholder,
  ...PHONE,
};

/** One stakeholder, six colleagues, in a wide overview column. */
export const OneStakeholderSeveralColleaguesWide: Story = {
  ...oneStakeholder,
  ...WIDE,
};

const aGap: Story = {
  args: {
    coverage: coverage({
      stakeholders: [tomas, ines],
      risks: [{ kind: "coverage_gap", summary: "no engaged champion" }],
    }),
    team: SIX_COLLEAGUES.slice(0, 2),
  },
  play: async ({ canvasElement }) => {
    const { theirs, legend } = await sides(canvasElement);
    await expect(
      theirs.getByText("Missing: an engaged champion"),
    ).toBeVisible();
    await expect(theirs.getByText("No role set · Not engaged")).toBeVisible();
    await expect(legend.getByText("Missing seat")).toBeVisible();
  },
};

/** No engaged champion: the gap names the seat, at phone width. */
export const AGapPhone: Story = { ...aGap, ...PHONE };

/** No engaged champion, in a wide overview column. */
export const AGapWide: Story = { ...aGap, ...WIDE };

const fullCommittee: Story = {
  args: {
    coverage: coverage(),
    team: SIX_COLLEAGUES.slice(0, 3).map((colleague) => ({
      ...colleague,
      carries: false,
    })),
  },
  play: async ({ canvasElement }) => {
    const { theirs, legend } = await sides(canvasElement);
    await expect(
      theirs.getByText("champion · Engaged · last wrote 2026-10-20"),
    ).toBeVisible();
    await expect(legend.queryByText("Missing seat")).toBeNull();
    await expect(legend.getAllByRole("listitem")).toHaveLength(3);
  },
};

/** Three seats, two engaged, nobody missing, at phone width. */
export const FullCommitteePhone: Story = { ...fullCommittee, ...PHONE };

/** Three seats, two engaged, nobody missing, in a wide overview column. */
export const FullCommitteeWide: Story = { ...fullCommittee, ...WIDE };

/**
 * A seat the reader may not name. The seat still counts toward coverage: how
 * many contacts carry a deal is not the fact being withheld, only who they are.
 */
export const SeatWithoutAName: Story = {
  args: {
    coverage: coverage({
      stakeholders: [dana, { ...ines, contact_name: null }],
    }),
    team: SIX_COLLEAGUES.slice(0, 1),
  },
  play: async ({ canvasElement }) => {
    const { theirs } = await sides(canvasElement);
    await expect(theirs.getAllByRole("listitem")).toHaveLength(2);
  },
};

/**
 * WITHHELD, not empty. Without the relationship grant a reader is served no
 * seats at all, and drawing that as an uncovered deal would be a finding from
 * a check that never ran.
 */
export const Withheld: Story = {
  args: {
    coverage: coverage({
      stakeholders: [],
      our_side: [],
      risks: [],
      sections_omitted: ["stakeholders", "our_side", "risks"],
    }),
    withheld: true,
    team: SIX_COLLEAGUES.slice(0, 1),
  },
  play: async ({ canvasElement }) => {
    const { theirs } = await sides(canvasElement);
    await expect(theirs.getByText("Hidden for your role")).toBeVisible();
  },
};

/** A deal with nobody recorded on their side. */
export const Empty: Story = {
  args: {
    coverage: coverage({ stakeholders: [] }),
    team: SIX_COLLEAGUES.slice(0, 1),
  },
  play: async ({ canvasElement }) => {
    const { theirs } = await sides(canvasElement);
    await expect(theirs.getByText("Nobody named")).toBeVisible();
  },
};
