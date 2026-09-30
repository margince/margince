// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useEffect } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { AiRuntimeChip, type AiRuntimeLabels } from "./airuntimechip";

type AiRunSummary = components["schemas"]["AiRunSummary"];

const LABELS: AiRuntimeLabels = {
  configured: "Configured AI",
  used: "Models used in this task",
  route: "Task · tier · provider",
  calls: "AI calls",
  tokens: "Tokens",
  latency: "Model latency",
  estimatedCost: "Estimated provider cost",
  partial: "Partial · unpriced usage exists",
  awaiting: "Shown after the first model call",
  unavailable: "Not available yet",
  chip: "Active model and cost",
  answering: "Active model",
  scope: "This run only. The full log is in Settings under AI.",
};

const RUNTIME: AiRunSummary = {
  currency: "USD",
  call_attempts: 4,
  tokens_in: 18_420,
  tokens_out: 2_615,
  latency_ms: 4_180,
  estimated_cost_microusd: 12_400,
  unpriced_calls: 0,
  models: [
    {
      task: "company-read",
      tier: "premium",
      provider: "deepseek",
      configured_model: "deepseek-chat",
      served_model: "deepseek-chat",
      call_attempts: 3,
      tokens_in: 16_100,
      tokens_out: 2_240,
      cached_tokens: 0,
      cache_write_tokens: 0,
      reasoning_tokens: 0,
      latency_ms: 3_450,
      estimated_cost_microusd: 11_100,
      unpriced_calls: 0,
      last_used_at: "2026-08-17T09:14:00Z",
    },
    {
      task: "extract",
      tier: "local-small",
      provider: "ollama",
      configured_model: "llama3.1:8b",
      served_model: "llama3.1:8b",
      call_attempts: 1,
      tokens_in: 2_320,
      tokens_out: 375,
      cached_tokens: 0,
      cache_write_tokens: 0,
      reasoning_tokens: 0,
      latency_ms: 730,
      estimated_cost_microusd: 1_300,
      unpriced_calls: 0,
      last_used_at: "2026-08-17T09:14:22Z",
    },
  ],
};

// The popover hangs off the chip's end, so the chip sits at the right edge
// with the height the popover drops into reserved.
const meta = {
  title: "Components/AI and provenance/AI runtime chip",
  component: AiRuntimeChip,
  parameters: { layout: "padded" },
  args: {
    configured: "deepseek-chat · llama3.1:8b",
    labels: LABELS,
    locale: "en",
  },
  decorators: [
    (Story) => (
      <div
        style={{
          display: "flex",
          justifyContent: "flex-end",
          alignItems: "flex-start",
          minHeight: "22rem",
        }}
      >
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof AiRuntimeChip>;
export default meta;

type Story = StoryObj<typeof meta>;

const openPopover: Story["play"] = async ({ canvasElement }) => {
  const chip = await within(canvasElement).findByRole("button", {
    name: /Active model and cost/,
  });
  await userEvent.click(chip);
  await expect(chip).toHaveAttribute("aria-expanded", "true");
};

export const AwaitingFirstCall: Story = {};

export const AfterARun: Story = {
  args: { runtime: RUNTIME },
  play: openPopover,
};

export const PartialEstimate: Story = {
  args: { runtime: { ...RUNTIME, unpriced_calls: 1 } },
  play: openPopover,
};

// The browser's text size, set on the root because every type token is in rem.
function LargeText({ children }: Readonly<{ children: ReactNode }>) {
  useEffect(() => {
    const root = document.documentElement;
    root.style.fontSize = "200%";
    return () => {
      root.style.fontSize = "";
    };
  }, []);
  return children;
}

// At 200% text the rows are taller than a phone, so the popover stops above
// the screen's foot and they scroll inside it, reachable by Tab.
export const PhoneLargeText: Story = {
  name: "phone — 200% text",
  args: { runtime: RUNTIME },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  decorators: [
    (Story) => (
      <LargeText>
        <Story />
      </LargeText>
    ),
  ],
  play: async (context) => {
    await openPopover?.(context);
    const region = await within(context.canvasElement).findByRole("region", {
      name: LABELS.answering,
    });
    // The popover fades in, so it is visible once the entrance has run.
    await waitFor(() => expect(region).toBeVisible());
  },
};
