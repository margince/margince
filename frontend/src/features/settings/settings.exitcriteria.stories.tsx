// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { StageExitCriteria } from "./settings.exitcriteria";

// What a stage asks for before a deal may leave it, as the pipelines card draws
// it inside one stage's row.

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
      <StageExitCriteria stageId={STAGE_ID} stageName="Discovery" canEdit />
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
      <StageExitCriteria stageId={STAGE_ID} stageName="Discovery" canEdit />
    </Served>
  ),
};

/** Two criteria, one that blocks the move and one that only counts toward it. */
export const RequiredAndOptional: Story = {
  render: () => (
    <Served
      answer={() =>
        jsonResponse({
          data: [
            criterion({}),
            criterion({
              id: "33333333-3333-4333-8333-333333333333",
              key: "champion_named",
              label: "A champion is named",
              kind: "role_identified",
              required: false,
              position: 2,
            }),
          ],
        })
      }
    >
      <StageExitCriteria stageId={STAGE_ID} stageName="Discovery" canEdit />
    </Served>
  ),
};

/** None yet: one line, and the verb that adds the first. */
export const Empty: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ data: [] })}>
      <StageExitCriteria stageId={STAGE_ID} stageName="Discovery" canEdit />
    </Served>
  ),
};

/** A reader sees each criterion and its standing, and no verb. */
export const ReadOnly: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ data: [criterion({})] })}>
      <StageExitCriteria
        stageId={STAGE_ID}
        stageName="Discovery"
        canEdit={false}
      />
    </Served>
  ),
};

export const RequiredAndOptionalDark: Story = {
  globals: { theme: "dark" },
  render: RequiredAndOptional.render,
};

/** The edit form names the key it keeps, in the code face, and offers no field for it. */
export const EditShowsKey: Story = {
  render: RequiredAndOptional.render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Edit Buyer confirmed the budget",
      }),
    );
    const dialog = within(
      await within(canvasElement.ownerDocument.body).findByRole("dialog"),
    );
    const key = await dialog.findByText("buyer_confirmed_budget");
    await expect(key.tagName).toBe("CODE");
    await expect(key.parentElement).toHaveTextContent(
      "Key: buyer_confirmed_budget",
    );
  },
};

export const EditShowsKeyPhone: Story = {
  ...EditShowsKey,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
