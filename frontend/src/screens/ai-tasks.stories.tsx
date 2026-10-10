// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { feature, status } from "./ai-admin.testkit";
import { AiTasksCard } from "./ai-tasks";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const decisionFirst = {
  ...feature,
  task: "capture_classify",
  display_name: "Classify correspondence",
  decision_first: true,
  decision_candidate: {
    tier: "decide",
    provider: "jev_compatible",
    model: "typesafe/jev-1.13",
    processing: "cloud_provider" as const,
  },
};

const premium = {
  ...feature,
  task: "draft_reply",
  display_name: "Draft a reply to an inbound thread with the account history",
  leading_tier: "premium",
};

const embeddings = {
  ...feature,
  task: "embeddings",
  display_name: "Search and retrieval",
  leading_tier: "embeddings",
  defaults: undefined,
};

function story() {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            allow: {
              ai_diagnostics: ["read"],
              ai_budget: ["read"],
              ai_routing: ["read"],
            },
          }),
        ),
      "GET /ai/status": () =>
        jsonResponse({
          ...status,
          features: [embeddings, premium, decisionFirst, feature],
        }),
      "GET /ai/health": () =>
        jsonResponse({
          window_hours: 1,
          rungs: [
            {
              tier: "decide",
              healthy: false,
              calls: 6,
              failures: 6,
              median_latency_ms: 430,
              last_sentinel: "decision_error",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <AiTasksCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiTasksCard> = {
  title: "Settings/AI/AI models/AI tasks",
  component: AiTasksCard,
};
export default meta;
type Story = StoryObj<typeof AiTasksCard>;

export const Default: Story = { render: story() };
export const Dark: Story = { render: story(), globals: { theme: "dark" } };

export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table");
  },
};
