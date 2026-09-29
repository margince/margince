// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import { AiBudgetCard, AiFeaturesCard } from "./ai-admin";
import { allowance, feature, status } from "./ai-admin.testkit";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

function story(
  band: "normal" | "degraded" | "queued",
  editable = true,
  features = status.features,
) {
  return () => {
    const budget = {
      ...allowance,
      band,
      spent_tokens:
        band === "queued"
          ? 26000000
          : band === "normal"
            ? 1000000
            : allowance.spent_tokens,
    };
    budget.remaining_tokens = Math.max(
      0,
      budget.monthly_tokens - budget.spent_tokens,
    );
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            allow: {
              ai_budget: editable ? ["read", "update"] : ["read"],
              ai_diagnostics: ["read"],
              ai_routing: ["read"],
            },
          }),
        ),
      "GET /ai/budget": () => jsonResponse(budget),
      "GET /ai/status": () => jsonResponse({ ...status, budget, features }),
      "POST /ai/budget/preview": () =>
        jsonResponse({
          current: budget,
          proposed: budget,
          features: status.features,
          deferred_work: status.deferred_work,
        }),
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <AiBudgetCard />
          <AiFeaturesCard />
        </div>
      </StoryProviders>
    );
  };
}
const meta: Meta<typeof AiBudgetCard> = {
  title: "Settings/AI/AI usage/Allowance",
  component: AiBudgetCard,
};
export default meta;
type Story = StoryObj<typeof AiBudgetCard>;
export const Degraded: Story = { render: story("degraded") };
export const Normal: Story = { render: story("normal") };
export const OverAllowance: Story = { render: story("queued") };
export const ReadOnly: Story = { render: story("degraded", false) };

export const DegradedDark: Story = {
  globals: { theme: "dark" },
  render: story("degraded"),
};
export const Phone: Story = { tags: ["uat-phone"], render: story("queued") };
export const Preview: Story = {
  render: story("degraded"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Summarize correspondence");
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit allowance" }),
    );
    await userEvent.click(
      await canvas.findByRole("button", { name: "Preview effects" }),
    );
  },
};

// Decision-first rows lead and carry their marker; a task that declares a
// decision form it cannot use follows; a two-rung ladder opens to the rungs
// behind its lead.
const [lead] = feature.effective_candidates;
const mixedFeatures = [
  { ...feature, task: "summarize" },
  {
    ...feature,
    task: "site_triage",
    display_name: "Triage a site",
    decision_skip_reason: "local_only" as const,
  },
  {
    ...feature,
    task: "capture_classify",
    display_name: "Classify correspondence",
    decision_first: true,
    decision_candidate: {
      tier: "decide",
      provider: "jev_compatible",
      model: "jev-classify",
      processing: "cloud_provider" as const,
    },
    effective_candidates: [
      lead,
      { ...lead, tier: "cheap_cloud", model: "fallback-model" },
    ],
  },
];
export const DecisionFirstOnTop: Story = {
  render: story("normal", true, mixedFeatures),
};
export const DecisionFirstOnTopDark: Story = {
  globals: { theme: "dark" },
  render: story("normal", true, mixedFeatures),
};
