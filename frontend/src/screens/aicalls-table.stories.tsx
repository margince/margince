// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { CallTable } from "./aicalls-table";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type CallSummary = components["schemas"]["AiCallSummary"];

const call: CallSummary = {
  id: "call-1",
  occurred_at: "2026-07-20T10:00:00Z",
  kind: "completion",
  task: "capture_classify",
  task_display_name: "Message classification",
  tier: "cheap_cloud",
  provider: "openai_compatible",
  model_id: "mistralai/mistral-small-2603",
  served_model: "mistralai/mistral-small-2603",
  calls_attempted: 1,
  tokens_in: 960,
  tokens_out: 8,
  reasoning_tokens: 0,
  cached_tokens: 0,
  latency_ms: 840,
  cache_hit: false,
  degraded: false,
  error_sentinel: null,
  has_payload: false,
  decision_attempted: false,
};

const CALLS: CallSummary[] = [
  call,
  {
    ...call,
    id: "call-2",
    task: "stage_evidence_extract",
    task_display_name: "Evidence extraction",
    calls_attempted: 2,
  },
  {
    ...call,
    id: "call-3",
    task: "weekly_review",
    task_display_name: undefined,
    tier: "premium",
    calls_attempted: 3,
    error_sentinel: "provider_quota",
  },
  {
    ...call,
    id: "call-4",
    task: "draft_reply",
    task_display_name: "Reply draft",
    error_sentinel: "metering_failed",
  },
];

const DETAIL = {
  ...call,
  served_identity_source: "echo",
  context_scopes: ["identity", "offer"],
  context_fingerprint: "abc",
  attempts: [
    {
      attempt: 1,
      is_terminal: true,
      kind: "completion",
      tier: "cheap_cloud",
      provider: "openai_compatible",
      model_id: "mistralai/mistral-small-2603",
      attempt_reason: "",
      tokens_in: 960,
      tokens_out: 8,
      latency_ms: 840,
      occurred_at: call.occurred_at,
    },
  ],
  payload_captured: false,
  payload: null,
};

function Calls() {
  installFetchStub({ "GET /ai/calls/call-1": () => jsonResponse(DETAIL) });
  return (
    <StoryProviders>
      <Panel title="AI call trace" className="aicalls-card">
        <CallTable calls={CALLS} captureEnabled={false} />
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof Calls> = {
  title: "Settings/AI/AI call log/Call table",
  component: Calls,
};
export default meta;
type Story = StoryObj<typeof Calls>;

export const Outcomes: Story = {};
export const OutcomesDark: Story = { globals: { theme: "dark" } };
export const OutcomesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    await within(canvasElement).findAllByRole("row");
    for (const row of canvasElement.querySelectorAll(
      "tbody tr:not([hidden])",
    )) {
      const box = (selector: string) =>
        row.querySelector(selector)?.closest("td")?.getBoundingClientRect();
      const model = box(".aicalls-model-line");
      const figures = box(".aicalls-figure");
      await expect(figures?.top).toBeGreaterThanOrEqual(model?.bottom ?? 0);
      const exact = row.querySelector(".aicalls-time")?.getBoundingClientRect();
      await expect(exact?.width).toBeLessThanOrEqual(1);
    }
  },
};
export const Opened: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: /Show attempts for Message classification/,
      }),
    );
    await canvas.findByRole("heading", { name: "Attempts" });
  },
};
