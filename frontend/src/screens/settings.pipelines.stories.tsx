// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { PipelinesCard } from "./settings.pipelines";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";

// The admin and read-only postures of this card live in settings.stories.tsx;
// this file holds the retired pipeline and its refused restore.
const meta: Meta = {
  title: "Settings/Sales/Pipelines/Retired pipeline",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const retiredPipeline = {
  id: "pl-old",
  name: "Legacy renewals",
  is_default: false,
  position: 1,
  archived_at: "2026-03-01T00:00:00Z",
  stages: [
    {
      id: "s1",
      pipeline_id: "pl-old",
      name: "Renew",
      position: 1,
      semantic: "open",
      win_probability: 60,
    },
  ],
};

export const RestoreRefused: Story = {
  render: () => {
    globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
    stubWithSession(
      {
        "GET /pipelines": () =>
          jsonResponse({
            data: [retiredPipeline],
            page: { next_cursor: null, has_more: false },
          }),
        "POST /pipelines/pl-old/restore": () =>
          jsonResponse(
            {
              type: "about:blank",
              title: "Restore refused",
              status: 409,
              detail: "A pipeline named Legacy renewals is already in use.",
            },
            409,
          ),
      },
      { pipeline: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <PipelinesCard />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Put back in use" }),
    );
    await expect(await canvas.findByRole("alert")).toHaveTextContent(
      "A pipeline named Legacy renewals is already in use.",
    );
  },
};
