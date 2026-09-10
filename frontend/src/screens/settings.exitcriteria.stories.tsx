// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { StageExitCriteria } from "./settings.exitcriteria";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a stage asks for before a deal may leave it, as the pipelines card draws
// it inside one stage's row. Three states are worth a picture: the standing
// notice about who evidence has to come from, the read that failed, and a
// terminal stage, which asks for nothing and says so.

type Criterion = components["schemas"]["StageExitCriterion"];

const STAGE_ID = "11111111-1111-4111-8111-111111111111";

function criterion(over: Partial<Criterion>): Criterion {
  return {
    id: "22222222-2222-4222-8222-222222222222",
    stage_id: STAGE_ID,
    key: "buyer_confirmed_budget",
    label: "Buyer confirmed the budget",
    kind: "buyer_confirmed",
    required: true,
    position: 1,
    ...over,
  };
}

const meta: Meta<typeof StageExitCriteria> = {
  title: "Settings/Sales/Pipelines/Stage exit criteria",
  component: StageExitCriteria,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageExitCriteria>;

// The card reads the stage's own route, so each story answers it rather than
// seeding a cache: data written with setQueryData is stale on arrival and the
// mount's background refetch would reach the network.
function Served({
  answer,
  children,
}: Readonly<{ answer: () => Response; children: ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({ pipeline: ["read", "create", "update"] }),
    [`GET /stages/${STAGE_ID}/exit-criteria`]: answer,
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/**
 * A criterion about something the BUYER did, which is what the standing notice
 * is for: no message our own side wrote can ever satisfy one.
 */
export const BuyerEvidence: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ data: [criterion({})] })}>
      <StageExitCriteria stageId={STAGE_ID} semantic="open" canEdit />
    </Served>
  ),
};

/**
 * The read failed. The list says nothing about what the stage holds — "nothing
 * yet" after a 500 would have an admin adding criteria that are already there.
 */
export const Unreadable: Story = {
  render: () => (
    <Served
      answer={() =>
        jsonResponse(
          {
            title: "Internal server error",
            status: 500,
            detail: "The pipelines store did not answer.",
          },
          500,
        )
      }
    >
      <StageExitCriteria stageId={STAGE_ID} semantic="open" canEdit />
    </Served>
  ),
};

/** Won and lost are where a deal stops, so there is nothing to configure. */
export const TerminalStage: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ data: [] })}>
      <StageExitCriteria stageId={STAGE_ID} semantic="won" canEdit />
    </Served>
  ),
};
