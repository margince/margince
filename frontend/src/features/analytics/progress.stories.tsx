// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { CONTEXTS } from "./analytics.overview.fixtures";
import { SalesGoalsScreen } from "./goals";
import { GoalMonths } from "./goals.bars";
import { GoalEditor } from "./goals.editor";
import { ProgressScreen } from "./progress";
import {
  draftPlan,
  goalRoster,
  publishedPlan,
  sellerMarch,
  teamMarch,
  workDeals,
} from "./progress.fixtures";
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
        <ProgressResult
          progress={sellerMarch}
          onEvidence={open}
          onGoals={open}
        />
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
        <ProgressResult progress={teamMarch} onEvidence={open} onGoals={open} />
        <ProgressMembers progress={teamMarch} />
      </div>
    </StoryProviders>
  ),
};

// A month nobody published a goal for: the hero says so once, with who can.
export const SellerNoGoal: Story = {
  render: () => (
    <StoryProviders>
      <div className="progress-body">
        <ProgressResult
          progress={{
            ...sellerMarch,
            goal: { status: "missing", months_set: 0, months_needed: 1 },
            completion: undefined,
            delta_minor: undefined,
          }}
          onEvidence={open}
          onGoals={open}
        />
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
      <GoalMonths months={publishedPlan.live_months} currency="EUR" />
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

const WORK_ROUTES = {
  "GET /leads": () =>
    jsonResponse({
      data: [lead],
      page: { next_cursor: "more", has_more: true, total: 4 },
    }),
  "GET /analytics/progress/deals": () =>
    jsonResponse({ data: workDeals, page: { has_more: false } }),
  "GET /companies": () =>
    jsonResponse({
      data: [
        { id: "company-nordwind", display_name: "Nordwind Logistik" },
        { id: "company-acme", display_name: "Acme Fördertechnik" },
      ],
      page: { has_more: false },
    }),
  "GET /users/names": () =>
    jsonResponse({
      data: [
        { id: teamMarch.members[0]?.owner_id, display_name: "Alex Brandt" },
        { id: teamMarch.members[1]?.owner_id, display_name: "Sam Okafor" },
      ],
    }),
};

// Open work of a team: leads by status, with the seller who owns each.
export const TeamWork: Story = {
  render: () => {
    installFetchStub(WORK_ROUTES);
    return (
      <StoryProviders>
        <ProgressWork progress={teamMarch} />
      </StoryProviders>
    );
  },
};

// The whole page as a seller opens it on their own March.
export const SellerPage: Story = {
  render: () => {
    installFetchStub({
      ...WORK_ROUTES,
      "GET /analytics/context": () => jsonResponse(CONTEXTS.sdr),
      "GET /analytics/progress": () => jsonResponse(sellerMarch),
    });
    window.location.hash = "#/progress?year=2026&month=3";
    return (
      <StoryProviders>
        <ProgressScreen />
      </StoryProviders>
    );
  },
};

// The whole page as a manager opens it on the team.
export const TeamPage: Story = {
  render: () => {
    installFetchStub({
      ...WORK_ROUTES,
      "GET /analytics/context": () => jsonResponse(CONTEXTS.leader),
      "GET /analytics/progress": () => jsonResponse(teamMarch),
    });
    window.location.hash = "#/progress?year=2026&month=3";
    return (
      <StoryProviders>
        <ProgressScreen />
      </StoryProviders>
    );
  },
};

// A manager's Sales goals: the sellers beside the draft in hand.
export const GoalsManager: Story = {
  render: () => {
    installFetchStub({
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
      [`GET /analytics/sales-goals/${draftPlan.owner_id}/2026`]: () =>
        jsonResponse(draftPlan),
    });
    window.location.hash = "#/goals?year=2026";
    return (
      <StoryProviders>
        <SalesGoalsScreen />
      </StoryProviders>
    );
  },
};

// A seller's Sales goals: their published year, to read and not to change.
export const GoalsSeller: Story = {
  render: () => {
    installFetchStub({
      "GET /analytics/sales-goals": () =>
        jsonResponse({ ...goalRoster, data: goalRoster.data.slice(0, 1) }),
      [`GET /analytics/sales-goals/${draftPlan.owner_id}/2026`]: () =>
        jsonResponse(publishedPlan),
    });
    window.location.hash = "#/goals?year=2026";
    return (
      <StoryProviders>
        <SalesGoalsScreen />
      </StoryProviders>
    );
  },
};

// A manager's view of a published year: text first, the editor on request.
export const GoalsPublished: Story = {
  render: () => {
    installFetchStub({
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
      [`GET /analytics/sales-goals/${draftPlan.owner_id}/2026`]: () =>
        jsonResponse({
          ...publishedPlan,
          capabilities: { edit: true },
          published: draftPlan.draft,
        }),
      "GET /analytics/progress": () => jsonResponse(sellerMarch),
    });
    window.location.hash = "#/goals?year=2026";
    return (
      <StoryProviders>
        <SalesGoalsScreen />
      </StoryProviders>
    );
  },
};

export const Phone: Story = {
  ...TeamPage,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
