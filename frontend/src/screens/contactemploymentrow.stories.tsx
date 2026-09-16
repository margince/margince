// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
// The row's verbs are the LIST's mutations — ending an employment and removing
// one are writes the list owns — so the row is drawn through its own list
// rather than through a hand-built stand-in for them. What is under review is
// still the row: the company it names, the inline role, the dates, and the
// verbs folded behind its overflow.
import { Employers } from "./contactemployers";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import "./contact360.css";

type Contact360 = components["schemas"]["Contact360"];

const contact: components["schemas"]["Contact"] = {
  id: "p-1",
  full_name: "Greta Schilling",
  writable: true,
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-01-05T09:00:00Z",
  updated_at: "2026-09-01T09:00:00Z",
};

const view: Contact360 = {
  as_of: "2026-09-01T09:00:00Z",
  contact,
  sections_omitted: [],
  employments: {
    data: [
      {
        relationship_id: "held",
        company_id: "o-1",
        company_name: "Brandt Automotive GmbH",
        role: "Head of Procurement",
        is_current_primary: true,
        employment_status: "current",
        started_at: "2021-03-01",
        started_precision: "month",
        version: 1,
      },
      {
        relationship_id: "ended",
        company_id: "o-2",
        company_name: "Nordwind Logistik",
        role: "Buyer",
        is_current_primary: false,
        employment_status: "former",
        started_at: "2016-08-01",
        ended_at: "2021-02-01",
        started_precision: "month",
        ended_precision: "month",
        version: 1,
      },
    ],
    page: { has_more: false },
  },
};

function Rows({ data }: Readonly<{ data: Contact360 }>) {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read", "update"],
      relationship: ["read", "update", "create", "delete"],
      company: ["read"],
    }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 420 }}>
        <Employers view={data} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Contact record/Employment row",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The two rows a career is made of: the job still held, which says so beside
// the company, and the one that ended, which carries its span instead.
export const HeldAndEnded: Story = {
  render: () => <Rows data={view} />,
};

// The row's own verbs, which stay folded until asked for: the row already
// carries a focusable inline-edit control, and a verb column standing open
// beside it would compete with the role a reader came here to correct. Ending
// the employment is offered only on the job still held.
export const VerbsOpen: Story = {
  render: () => <Rows data={view} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const [first] = await canvas.findAllByRole("button", {
      name: "More actions",
    });
    await userEvent.click(first);
  },
};

// A reader who may not change the record: the career reads the same and the
// overflow is gone, rather than offering verbs the save would refuse.
export const ReadOnly: Story = {
  render: () => (
    <Rows data={{ ...view, contact: { ...contact, writable: false } }} />
  ),
};
