// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { AiCallsCard } from "./aicalls";
import { CallDetailPanel } from "./aicalls-detail";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The card is gated on ai_diagnostics:read, so /me decides which of its two
// branches renders. Left unrouted, the fetch stub answers with an empty list
// page, useMe rejects that as malformed, and every grant fails closed — which
// is how the List and Empty stories below both used to draw the same probe
// error under two names that promised the trace table.
const OPERATOR: GrantSpec = { ai_diagnostics: ["read"] };

const summary = {
  id: "call-1",
  occurred_at: "2026-07-20T10:00:00Z",
  kind: "completion",
  task: "capture_classify",
  tier: "cheap_cloud",
  provider: "gemini",
  model_id: "configured",
  served_model: "served",
  calls_attempted: 2,
  tokens_in: 100,
  tokens_out: 20,
  reasoning_tokens: 0,
  cached_tokens: 0,
  latency_ms: 900,
  cache_hit: false,
  degraded: true,
  error_sentinel: "provider_unavailable",
  has_payload: true,
  decision_attempted: false,
};
const detail = {
  ...summary,
  served_identity_source: "response",
  context_scopes: ["identity"],
  context_fingerprint: "abc",
  attempts: [
    {
      attempt: 1,
      is_terminal: false,
      kind: "completion",
      attempt_reason: "",
      tokens_in: 100,
      tokens_out: 0,
      latency_ms: 400,
      occurred_at: summary.occurred_at,
    },
    {
      attempt: 2,
      is_terminal: true,
      kind: "completion",
      attempt_reason: "retry_on_5xx",
      tokens_in: 100,
      tokens_out: 20,
      latency_ms: 900,
      occurred_at: summary.occurred_at,
    },
  ],
  payload_captured: true,
  payload: { request: { system: "safe", messages: [] }, response: "ok" },
};

// Calls as a real installation writes them: a broker-served model, whose id is
// one long unbreakable string, on the tasks with the longest names.
const brokered = [
  {
    ...summary,
    id: "call-2",
    task: "capture_counterparty_verdict",
    tier: "premium",
    provider: "openai_compatible",
    served_model: "mistralai/mistral-small-2603",
    tokens_in: 1250344,
    tokens_out: 84213,
    latency_ms: 12873,
    degraded: false,
    error_sentinel: "",
    decision_attempted: true,
  },
  {
    ...summary,
    id: "call-3",
    task: "stage_evidence_extract",
    tier: "cheap_cloud",
    provider: "openai_compatible",
    served_model: "google/gemini-3.1-flash-lite-preview-09-2026",
    tokens_in: 48211,
    tokens_out: 3120,
    latency_ms: 2210,
    degraded: false,
    error_sentinel: "",
  },
];

// One row per outcome the column draws: none for a first-time answer, a
// warning for a call that limped, danger for no answer.
const outcomes = [
  {
    ...summary,
    id: "call-ok",
    task: "weekly_review",
    task_display_name: "Weekly review narrative",
    tier: "premium",
    calls_attempted: 1,
    degraded: false,
    error_sentinel: null,
    cache_hit: true,
  },
  {
    ...summary,
    id: "call-retried",
    task_display_name: "Message classification",
    degraded: false,
    error_sentinel: null,
  },
  { ...summary, id: "call-degraded", error_sentinel: null },
  {
    ...summary,
    id: "call-rejected",
    task: "owed_verdict",
    task_display_name: "Unanswered-message triage",
    calls_attempted: 3,
    degraded: false,
    error_sentinel: "output_rejected",
  },
  {
    ...summary,
    id: "call-decided",
    kind: "decision",
    tier: "decide",
    provider: "jev_compatible",
    served_model: "typesafe/jev-1.13",
    calls_attempted: 1,
    degraded: false,
    error_sentinel: null,
    decision_attempted: true,
  },
];

function list(
  data: unknown[],
  capture = true,
  allow: GrantSpec = OPERATOR,
  trace: unknown = detail,
) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/calls": () =>
        jsonResponse({
          data,
          page: { has_more: false },
          payload_capture_enabled: capture,
          tasks: ["capture_classify"],
          task_options: [
            {
              task: "capture_classify",
              display_name: "Message classification",
            },
          ],
        }),
      "GET /ai/calls/call-1": () => jsonResponse(trace),
    });
    return (
      <StoryProviders>
        <AiCallsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AiCallsCard> = {
  title: "Settings/AI/AI call log/AI call log",
  component: AiCallsCard,
};
export default meta;
type Story = StoryObj<typeof AiCallsCard>;
export const List: Story = { render: list([summary]) };
export const Outcomes: Story = { render: list(outcomes) };
export const OutcomesDark: Story = {
  globals: { theme: "dark" },
  render: list(outcomes),
};
export const OutcomesPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: list(outcomes),
};
export const Empty: Story = { render: list([]) };

// No ai_diagnostics grant: the trace keeps its place and says it is withheld. An
// absent card would read as "this installation made no model calls".
export const Withheld: Story = { render: list([summary], true, {}) };

// A page that has a next one, which is the only state that draws the pager. The
// trace row stacks its control as a COLUMN — `.settingrow-control` is a flex row
// — so this is the story that says whether Load more sits under the table or
// beside it.
export const ListPaged: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow: OPERATOR })),
      "GET /ai/calls": () =>
        jsonResponse({
          data: [summary],
          page: { has_more: true, next_cursor: "page-2" },
          payload_capture_enabled: true,
          tasks: ["capture_classify"],
          task_options: [
            {
              task: "capture_classify",
              display_name: "Message classification",
            },
          ],
        }),
      "GET /ai/calls/call-1": () => jsonResponse(detail),
    });
    return (
      <StoryProviders>
        <AiCallsCard />
      </StoryProviders>
    );
  },
};

export const PayloadOff: Story = {
  render: () => {
    installFetchStub({
      "GET /ai/calls/call-1": () =>
        jsonResponse({ ...detail, payload_captured: false, payload: null }),
    });
    return (
      <StoryProviders>
        <CallDetailPanel id="call-1" captureEnabled={false} />
      </StoryProviders>
    );
  },
};
export const WithPayload: Story = {
  render: () => {
    installFetchStub({ "GET /ai/calls/call-1": () => jsonResponse(detail) });
    return (
      <StoryProviders>
        <CallDetailPanel id="call-1" captureEnabled />
      </StoryProviders>
    );
  },
};

// The disclosure button opens the attempt trail under its own row, so the trace
// stays readable as one thing rather than two surfaces side by side. Shared by
// the stories below, which all need the row OPEN to show what they are about.
const openAttemptTrail: NonNullable<Story["play"]> = async ({
  canvasElement,
}) => {
  const canvas = within(canvasElement);
  const disclosure = await canvas.findByRole("button", {
    name: /Show attempts for capture_classify/,
  });
  await userEvent.click(disclosure);
  await canvas.findByRole("heading", { name: "Attempts" });
};

// The detail panel IN the table, which is the only place a reader meets it.
export const RowExpanded: Story = {
  render: list([summary]),
  play: openAttemptTrail,
};

// A press anywhere on the row opens it as the chevron does.
export const RowPressed: Story = {
  render: list([summary]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: /Show attempts/ });
    await userEvent.click(canvas.getByText("capture_classify"));
    await canvas.findByRole("heading", { name: "Attempts" });
  },
};

export const RowExpandedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: list([summary]),
  play: openAttemptTrail,
};

// A binding that names no model, as the offline fake does: the served line
// says what answered and leaves the configured model out.
export const RowExpandedNoConfiguredModel: Story = {
  render: list([{ ...summary, model_id: "" }], true, OPERATOR, {
    ...detail,
    model_id: "",
  }),
  play: openAttemptTrail,
};

// The same expanded row in dark. Two badge tones are all that separates a call
// that limped from one that failed — `degraded` (warning) and the error sentinel
// (danger) — so a tint that stops carrying that distinction against a dark card
// takes the task column's meaning with it. The trail is open because the
// attempt table brings a second danger badge onto a nested surface, where a
// translucent tone composites over a different ground than on the card face.
export const RowExpandedDark: Story = {
  globals: { theme: "dark" },
  render: list([summary]),
  play: openAttemptTrail,
};

// At 390px the trace folds each call onto two lines. The task, its outcome and
// its toggle come first; the moment, model, tokens and latency sit under them.
export const ListPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: list([summary]),
};

// The card at the width Settings gives it — a page column beside the settings
// navigation, not the viewport — with the long model ids a broker serves. Every
// column has to be reachable here without a sideways scroll: an overlay
// scrollbar draws nothing, so a table wider than its card simply looks cut off.
export const BrokerModelsAtSettingsWidth: Story = {
  render: () => {
    const Card = list(brokered);
    return (
      <div style={{ maxWidth: 820 }} data-testid="settings-width">
        <Card />
      </div>
    );
  },
  // The assertion that would have shown the clipping: the scroll box holds its
  // table, so nothing is waiting off to the right.
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/mistral-small-2603/);
    const scroller = canvasElement.querySelector(".table-scroll");
    expect(scroller?.scrollWidth).toBeLessThanOrEqual(
      scroller?.clientWidth ?? 0,
    );
  },
};

export const BrokerModelsPhone: Story = {
  ...BrokerModelsAtSettingsWidth,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
