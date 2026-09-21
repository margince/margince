// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import type { GrantSpec } from "../app/mefixture";
import { TeamPlanReview } from "./brief.teamplan";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// What a lead sees of a teammate's week, opened from the team board's own
// verb. Every instant is FIXED: a plan built from `new Date()` documents
// whichever day the catalog happened to be opened on.
//
// The state that matters here is whether the lead may ANSWER. A help request
// with a reply box and the same request with the reply merely quoted are the
// same row read by two seats, and the plan's own `status` decides it as much
// as the grant does — a closed week takes no answers from anyone.

type WeeklyPlan = components["schemas"]["WeeklyPlan"];

const OWNER = "01930000-0000-7000-8000-0000000000u2";
const PLAN_ROUTE = `GET /weekly-plans/${OWNER}/current`;

const PLAN: WeeklyPlan = {
  id: "01930000-0000-7000-8000-0000000000p1",
  local_week_start: "2026-08-10",
  status: "open",
  commitments: [
    {
      id: "01930000-0000-7000-8000-0000000000k1",
      label: "Get the Brandt renewal signed",
      state: "open",
      due_on: "2026-08-14",
      help_requested:
        "Their legal team wants a clause we have never agreed to. Can you look?",
      position: 1,
    },
    {
      id: "01930000-0000-7000-8000-0000000000k2",
      label: "Three discovery calls in the new segment",
      state: "done",
      position: 2,
    },
    {
      id: "01930000-0000-7000-8000-0000000000k3",
      label: "Rewrite the onboarding follow-up sequence",
      state: "dropped",
      position: 3,
    },
  ],
};

function review(routes: RouteMap, allow: GrantSpec) {
  return () => {
    stubWithSession(routes, allow);
    return (
      <StoryProviders>
        <TeamPlanReview owner={OWNER} name="Mara" />
      </StoryProviders>
    );
  };
}

// The dialog only exists once the lead asks for it, so the story presses the
// same button they do. It portals to the document body, so the click is all
// this asks of the canvas.
const openThePlan = async ({
  canvasElement,
}: {
  canvasElement: HTMLElement;
}) => {
  const user = userEvent.setup();
  const canvas = within(canvasElement);
  await user.click(
    await canvas.findByRole("button", { name: "Review current plan" }),
  );
};

const meta: Meta<typeof TeamPlanReview> = {
  title: "Shell/Home team plan",
  component: TeamPlanReview,
};
export default meta;

type Story = StoryObj<typeof TeamPlanReview>;

const planRoutes: RouteMap = { [PLAN_ROUTE]: () => jsonResponse(PLAN) };

// A lead who may answer, over an open week: the help request carries a reply
// box, and Save stays refused until they have written something other than
// what already stands.
export const HelpAskedAndAnswerable: Story = {
  render: review(planRoutes, { weekly_plan: ["read", "update"] }),
  play: openThePlan,
};

// The same week read by a colleague who holds no answer grant. The request is
// still shown — knowing what a teammate is stuck on is not a write — and the
// answer is quoted rather than offered as a field.
export const HelpAskedButNotTheirs: Story = {
  render: review(planRoutes, { weekly_plan: ["read"] }),
  play: openThePlan,
};

// The week is closed, so nobody answers: a closed plan stops accepting edits,
// which is what keeps the review's counts true.
export const ClosedWeek: Story = {
  render: review(
    { [PLAN_ROUTE]: () => jsonResponse({ ...PLAN, status: "closed" }) },
    { weekly_plan: ["read", "update"] },
  ),
  play: openThePlan,
};

// This teammate has not opened a week. A 404 is that absence and not a
// failure, so the dialog says so in their name rather than drawing a refusal.
export const NoPlanThisWeek: Story = {
  render: review(
    {
      [PLAN_ROUTE]: () =>
        jsonResponse({ title: "Not found", status: 404 }, 404),
    },
    { weekly_plan: ["read", "update"] },
  ),
  play: openThePlan,
};
