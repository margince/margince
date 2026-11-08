// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { type Goal, GoalFields, goalOf } from "./reporting.initiativegoal";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

// The initiative drawer's goal step: what the initiative aims at, by when, and
// who owns it. A new one names no owner and reads as the caller's.

const meta: Meta = {
  title: "Records/Reports/Analytics/Initiative goal",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function Fields({ initial }: Readonly<{ initial: Goal }>) {
  const [goal, setGoal] = useState(initial);
  return (
    <GoalFields goal={goal} onChange={setGoal} onProposal={() => undefined} />
  );
}

const roster = {
  data: [
    { id: "u-maya", email: "maya@acme.test", display_name: "Maya Lindqvist" },
    { id: "u-sam", email: "sam@acme.test", display_name: "Sam Okafor" },
  ],
  page: { next_cursor: null, has_more: false },
};

function step(initial: Goal) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /users": () => jsonResponse(roster),
  });
  return (
    <StoryProviders>
      <Fields initial={initial} />
    </StoryProviders>
  );
}

/** A new initiative: no owner named, so the caller owns it. */
export const NewInitiative: Story = { render: () => step(goalOf()) };

/** An initiative a teammate owns. */
export const OwnedByATeammate: Story = {
  render: () =>
    step({
      ...goalOf(),
      name: "DACH expansion",
      deadline: "2026-12-31",
      ownerId: "u-sam",
    }),
};
