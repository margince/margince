// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { feature } from "../../testing/ai-admin.testkit";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AiFeatureTable } from "./ai-feature-table";

const ROWS = [
  {
    ...feature,
    task: "embeddings",
    display_name: "Search and retrieval",
    leading_tier: "embeddings",
    defaults: undefined,
  },
  {
    ...feature,
    task: "draft_reply",
    display_name: "Draft a reply to an inbound thread with the account history",
    leading_tier: "premium",
  },
  feature,
  {
    ...feature,
    task: "triage",
    display_name: "Triage a site",
    leading_tier: "local_small",
    impact: "model_changed" as const,
  },
];

function Tasks({ editable }: Readonly<{ editable: boolean }>) {
  installFetchStub({ "GET /me": () => jsonResponse(meFixture({ allow: {} })) });
  return (
    <StoryProviders>
      <Panel title="AI tasks">
        <AiFeatureTable
          rows={ROWS}
          canManage
          onEdit={editable ? () => {} : undefined}
        />
      </Panel>
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/AI models/Task table",
  component: Tasks,
  args: { editable: true },
} satisfies Meta<typeof Tasks>;
export default meta;
type Story = StoryObj<typeof meta>;

// One table per tier, headed by its name, smallest tier first.
export const InTasksPanel: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const tables = await canvas.findAllByRole("table");
    if (tables[0]?.getAttribute("aria-label") !== "Small local") {
      throw new Error("the first group is not the smallest tier");
    }
  },
};

export const InBudgetPreview: Story = { args: { editable: false } };

export const Dark: Story = { globals: { theme: "dark" } };

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table", { name: "Premium" });
  },
};
