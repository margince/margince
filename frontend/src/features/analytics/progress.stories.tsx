// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { deals } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { MonthsTable } from "./goals";
import { GoalEditor } from "./goals.editor";
import { draftPlan, sellerMarch, teamMarch } from "./progress.fixtures";
import { ProgressReadings, ProgressResult } from "./progress.result";
import { ProgressMembers, ProgressMonths } from "./progress.tables";
import { ProgressWork } from "./progress.work";

const meta: Meta = { title: "Records/Reports/Progress" };
export default meta;

type Story = StoryObj;

const open = () => undefined;

// A seller short of their goal: danger delta, capped meter, readings.
export const SellerMonth: Story = {
  render: () => (
    <StoryProviders>
      <div className="progress-body">
        <ProgressResult progress={sellerMarch} onEvidence={open} />
        <ProgressReadings progress={sellerMarch} onEvidence={open} />
        <ProgressMonths progress={sellerMarch} />
      </div>
    </StoryProviders>
  ),
};

// A team with one seller still without a goal: no comparison, the subtotal.
export const TeamIncomplete: Story = {
  render: () => (
    <StoryProviders>
      <div className="progress-body">
        <ProgressResult progress={teamMarch} onEvidence={open} />
        <ProgressMembers progress={teamMarch} />
      </div>
    </StoryProviders>
  ),
};

// A manager's editor over a saved draft that may be published.
export const GoalDraft: Story = {
  render: () => (
    <StoryProviders>
      <GoalEditor plan={draftPlan} />
    </StoryProviders>
  ),
};

// What a seller reads: the published months and their sum.
export const PublishedMonths: Story = {
  render: () => (
    <StoryProviders>
      <MonthsTable months={draftPlan.draft?.months ?? []} currency="EUR" />
    </StoryProviders>
  ),
};

const lead: components["schemas"]["Lead"] = {
  id: "l-1",
  full_name: "Jonas Petersen",
  company_name: "Nordwind Logistik",
  status: "new",
  score: 41,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-24T11:00:00Z",
  next_task_subject: "Call about the fleet size",
  next_task_due_at: "2026-08-26T09:00:00Z",
};

// Open work of a team: leads by status, with the seller who owns each.
export const TeamWork: Story = {
  render: () => {
    installFetchStub({
      "GET /leads": () =>
        jsonResponse({
          data: [lead],
          page: { next_cursor: "more", has_more: true },
        }),
      "GET /deals": () =>
        jsonResponse({ data: deals, page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <ProgressWork progress={teamMarch} />
      </StoryProviders>
    );
  },
};

export const Phone: Story = {
  ...SellerMonth,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
