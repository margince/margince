// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { feature, status } from "./ai-admin.testkit";
import { AiTasksCard } from "./ai-tasks";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// What each AI task runs on now, read-only, with how the lane it starts on is
// answering. The decision-first tasks lead and read top to bottom.

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
        jsonResponse({ ...status, features: [decisionFirst, feature] }),
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
  title: "Settings/AI/Models and routing/AI tasks",
  component: AiTasksCard,
};
export default meta;
type Story = StoryObj<typeof AiTasksCard>;

export const Default: Story = { render: story() };
export const Dark: Story = { render: story(), globals: { theme: "dark" } };
