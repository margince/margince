// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadNextBar } from "./leadnext";
import type { Lead } from "./leads";

type Activity = components["schemas"]["Activity"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u-1",
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
  next_task_subject: "Send the depot quote",
  next_task_due_at: "2026-06-22T09:00:00Z",
  open_task_count: 2,
};

function task(id: string, subject: string, dueAt: string): Activity {
  return {
    id,
    kind: "task",
    subject,
    due_at: dueAt,
    occurred_at: "2026-06-18T08:00:00Z",
    occurred_precision: "instant",
    offers_email: false,
    source: "manual",
    captured_by: "human:u-1",
    is_done: false,
    created_at: "2026-06-18T08:00:00Z",
    updated_at: "2026-06-18T08:00:00Z",
  };
}

const meta = {
  title: "Records/Leads/Next bar",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj;

// The bar names the lead's own next task and counts the rest behind it.
export const NextTask: Story = {
  render: () => {
    installFetchStub({ "GET /me": meRoute({ lead: ["read", "update"] }) });
    return (
      <StoryProviders>
        <LeadNextBar
          lead={lead}
          asOf="2026-06-20T15:00:00Z"
          activities={[
            task("a-1", "Send the depot quote", "2026-06-22T09:00:00Z"),
            task("a-2", "Call about the fleet size", "2026-06-25T09:00:00Z"),
          ]}
          onQualify={() => undefined}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("Send the depot quote"),
    ).toBeVisible();
  },
};
