// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { focusManager } from "@tanstack/react-query";
import { expect, userEvent, within } from "storybook/test";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { AiBudgetCard } from "./ai-admin";
import { allowance, feature, status } from "./ai-admin.testkit";

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
        </div>
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Settings/AI/AI usage/Allowance",
  component: AiBudgetCard,
} satisfies Meta<typeof AiBudgetCard>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Degraded: Story = { render: story("degraded") };
export const Normal: Story = { render: story("normal") };
export const OverAllowance: Story = { render: story("queued") };
export const ReadOnly: Story = { render: story("degraded", false) };

export const DegradedDark: Story = {
  globals: { theme: "dark" },
  render: story("degraded"),
};
export const Phone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story("queued"),
};
// The normal band carries no callout: the meter already says the month is fine.
export const NormalPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story("normal"),
};
export const Preview: Story = {
  render: story("degraded"),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit allowance" }),
    );
    await userEvent.click(
      await canvas.findByRole("button", { name: "Preview effects" }),
    );
  },
};

// Another admin saved while this one edited: the editor says so and offers the
// saved values back.
export const ChangedWhileEditing: Story = {
  render: () => {
    let reads = 0;
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { ai_budget: ["read", "update"] } })),
      "GET /ai/budget": () => {
        reads += 1;
        return jsonResponse(
          reads === 1
            ? allowance
            : {
                ...allowance,
                config: {
                  tokens_per_full_user: 15000000,
                  company_monthly_tokens: null,
                },
                revision: "budget-v2",
              },
        );
      },
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          <AiBudgetCard />
        </div>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit allowance" }),
    );
    // A window coming back into focus reads the allowance again.
    focusManager.setFocused(false);
    focusManager.setFocused(true);
    await expect(
      await canvas.findByRole("button", { name: "Reload values" }),
    ).toBeInTheDocument();
    focusManager.setFocused(undefined);
  },
};

// Decision-first rows lead and carry their marker; a task that declares a
// decision form it cannot use follows; a two-rung ladder opens to the rungs
// behind its lead.
const [lead] = feature.effective_candidates;
if (!lead) {
  throw new Error("the feature fixture carries no candidate to lead with");
}
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
