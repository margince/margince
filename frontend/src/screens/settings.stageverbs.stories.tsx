// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { StageMenu } from "./settings.stageverbs";
import { StoryProviders, stubWithSession } from "./story-utils";

// One stage's row menu on its own. A closing stage offers only Edit, because the
// server refuses criteria and removal there.

type Stage = components["schemas"]["Stage"];

const open: Stage = {
  id: "66666666-6666-4666-8666-666666666661",
  pipeline_id: "33333333-3333-4333-8333-333333333333",
  name: "Proposal",
  position: 3,
  semantic: "open",
  win_probability: 50,
  version: 2,
  deal_count: 0,
};

const won: Stage = { ...open, name: "Closed won", semantic: "won" };

function served(stage: Stage) {
  return () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    stubWithSession({}, { pipeline: ["read", "update", "delete"] });
    return (
      <StoryProviders>
        <StageMenu
          stage={stage}
          verbs={{
            writable: true,
            canRemove: true,
            onAddCriterion: fn(),
            onRemove: fn(),
          }}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof StageMenu> = {
  title: "Settings/Sales/Pipelines/Stage row menu",
  component: StageMenu,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageMenu>;

/** An open stage: edit it, ask a criterion of it, or remove it. */
export const OpenStage: Story = {
  render: served(open),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Proposal" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      page.getByRole("button", { name: "Edit stage" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Add criterion" }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Remove" })).toBeEnabled();
  },
};

/** A closing stage: edit alone. */
export const ClosingStage: Story = {
  render: served(won),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Closed won" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      page.getByRole("button", { name: "Edit stage" }),
    ).toBeVisible();
    await expect(page.queryByRole("button", { name: "Remove" })).toBeNull();
  },
};
