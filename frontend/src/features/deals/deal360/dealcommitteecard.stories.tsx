// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import type { components } from "../../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../../testing/story-utils";
import { DealCommitteeCard } from "./dealcommitteecard";

// The committee card as the deal Overview mounts it. Our team comes from the
// deal's responsibilities and their team from the coverage read. A gap carries
// the verb that seats a stakeholder for it.

type DealCoverage = components["schemas"]["DealCoverage"];
type RecordAssignment = components["schemas"]["RecordAssignment"];

const DEAL = "01a03000-0000-7000-8000-00000000d001";
const OWNER = "01a03000-0000-7000-8000-00000000c001";

const meta: Meta<typeof DealCommitteeCard> = {
  title: "Records/Deal 360/Committee card",
  component: DealCommitteeCard,
};
export default meta;

type Story = StoryObj<typeof DealCommitteeCard>;

function held(subject: string, name: string, role: string): RecordAssignment {
  return {
    id: `a-${subject}`,
    record_type: "deal",
    record_id: DEAL,
    subject_kind: "user",
    subject_id: subject,
    subject_name: name,
    subject_inactive: false,
    role_id: `r-${subject}`,
    role_key: role.toLowerCase().replaceAll(" ", "_"),
    role_label: role,
    role_active: true,
    version: 1,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
  };
}

const TEAM = [
  held(OWNER, "Lena Fischer", "Sales owner"),
  held("u2", "Tomás Reyes", "Delivery owner"),
  held("u3", "Ana Lima", "Solution engineer"),
  held("u4", "Kai Brandt", "Legal counsel"),
];

const champion: DealCoverage["stakeholders"][number] = {
  contact_id: "c1",
  contact_name: "Mai Trần",
  role: "champion",
  engaged: true,
  last_inbound_at: "2026-10-21T08:30:00Z",
  last_inbound_precision: "instant",
};

function view(over: Partial<DealCoverage>): DealCoverage {
  return {
    deal_id: DEAL,
    stakeholders: [champion],
    our_side: [],
    risks: [],
    sections_omitted: [],
    ...over,
  };
}

function Card({
  coverage,
  width,
}: Readonly<{ coverage: DealCoverage; width?: number }>): ReactNode {
  stubWithSession(
    {
      "GET /users/names": () =>
        jsonResponse({ data: [{ id: OWNER, display_name: "Lena Fischer" }] }),
      [`GET /records/deal/${DEAL}/assignments`]: () =>
        jsonResponse({ data: TEAM }),
    },
    { relationship: ["read", "create"] },
  );
  return (
    <StoryProviders>
      <div
        style={width ? { inlineSize: width } : { padding: "var(--padCard)" }}
      >
        <DealCommitteeCard
          dealId={DEAL}
          ownerId={OWNER}
          coverage={coverage}
          withheld={false}
          pending={false}
          roleLabel={(role) => role.replaceAll("_", " ")}
        />
      </div>
    </StoryProviders>
  );
}

// One column of the deal overview at a 1440px window.
const WIDE_COLUMN = 520;
const PHONE = {
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
const WIDE = { tags: ["uat-wide"] };

const oneStakeholder = view({
  risks: [
    { kind: "single_threaded_theirs", summary: "one thread" },
    {
      kind: "single_threaded_ours",
      summary: "one colleague carries almost all the contact",
      user_ids: ["u3"],
    },
  ],
});

async function expectFourColleagues(canvasElement: HTMLElement) {
  const ours = within(
    await within(canvasElement).findByRole("region", { name: "Your team" }),
  );
  await ours.findByText("Kai Brandt");
  await expect(ours.getAllByRole("listitem")).toHaveLength(4);
  await expect(ours.getByText("Owner, Sales owner")).toBeVisible();
  const carrier = ours.getByText("Handles most exchanges").closest("li");
  await expect(carrier).toHaveTextContent("Ana Lima");
}

/** One stakeholder and four colleagues, one of whom handles most exchanges. */
export const OneStakeholderSeveralColleaguesPhone: Story = {
  ...PHONE,
  render: () => <Card coverage={oneStakeholder} />,
  play: async ({ canvasElement }) => expectFourColleagues(canvasElement),
};

export const OneStakeholderSeveralColleaguesWide: Story = {
  ...WIDE,
  render: () => <Card coverage={oneStakeholder} width={WIDE_COLUMN} />,
  play: async ({ canvasElement }) => expectFourColleagues(canvasElement),
};

const aGap = view({
  stakeholders: [{ ...champion, role: "user", engaged: false }],
  risks: [{ kind: "coverage_gap", summary: "no engaged champion" }],
});

// The gap's own verb opens the stakeholder form on the seat it misses.
async function fillTheGap(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  const gap = await canvas.findByText("Missing: an engaged champion");
  const verb = (
    await canvas.findAllByRole("button", { name: "Add stakeholder" })
  ).find((button) => button.getAttribute("aria-describedby") === gap.id);
  await expect(verb).toBeDefined();
  if (verb) {
    await userEvent.click(verb);
  }
  const dialog = await screen.findByRole("dialog");
  await expect(
    within(dialog).getByRole("textbox", { name: "Role" }),
  ).toHaveValue("champion");
}

export const AGapPhone: Story = {
  ...PHONE,
  render: () => <Card coverage={aGap} />,
  play: async ({ canvasElement }) => fillTheGap(canvasElement),
};

export const AGapWide: Story = {
  ...WIDE,
  render: () => <Card coverage={aGap} width={WIDE_COLUMN} />,
  play: async ({ canvasElement }) => fillTheGap(canvasElement),
};

const full = view({
  stakeholders: [
    champion,
    {
      contact_id: "c2",
      contact_name: "Bảo Nguyễn",
      role: "economic_buyer",
      engaged: true,
      last_inbound_at: "2026-10-09T12:00:00Z",
      last_inbound_precision: "day",
    },
    {
      contact_id: "c3",
      contact_name: "Linh Phạm",
      role: "influencer",
      engaged: false,
    },
  ],
});

async function expectNoGap(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await canvas.findByText("Bảo Nguyễn");
  await expect(canvas.queryByText(/^Missing:/)).toBeNull();
  await expect(canvas.getByText("3 named · 2 of 3 engaged")).toBeVisible();
}

export const FullCommitteePhone: Story = {
  ...PHONE,
  render: () => <Card coverage={full} />,
  play: async ({ canvasElement }) => expectNoGap(canvasElement),
};

export const FullCommitteeWide: Story = {
  ...WIDE,
  render: () => <Card coverage={full} width={WIDE_COLUMN} />,
  play: async ({ canvasElement }) => expectNoGap(canvasElement),
};
