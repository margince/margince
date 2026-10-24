// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { translate } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AiCallsCard } from "./aicalls";

// The card is gated on ai_diagnostics:read, so /me decides which branch renders.
const OPERATOR: GrantSpec = { ai_diagnostics: ["read"] };

const summary = {
  id: "call-1",
  occurred_at: "2026-07-20T10:00:00Z",
  kind: "completion",
  task: "capture_classify",
  tier: "cheap_cloud",
  provider: "gemini",
  model_id: "gemini-2.5-flash",
  served_model: "gemini-2.5-flash",
  calls_attempted: 2,
  tokens_in: 1840,
  tokens_out: 212,
  reasoning_tokens: 0,
  cached_tokens: 0,
  latency_ms: 900,
  cache_hit: false,
  degraded: false,
  error_sentinel: null,
  has_payload: true,
  decision_attempted: false,
  outcome: "fallback",
  estimated_cost_microusd: 3612,
};

// One call per outcome bucket, so the outcome dial has every bucket to find.
const calls = [
  summary,
  {
    ...summary,
    id: "call-2",
    occurred_at: "2026-07-20T09:58:00Z",
    task: "enrich",
    tier: "premium",
    provider: "anthropic",
    model_id: "claude-sonnet",
    served_model: "claude-sonnet",
    calls_attempted: 1,
    tokens_in: 5210,
    tokens_out: 940,
    latency_ms: 3120,
    outcome: "answered",
    estimated_cost_microusd: 29_730,
  },
  {
    ...summary,
    id: "call-3",
    occurred_at: "2026-07-20T09:51:00Z",
    task: "weekly_review",
    calls_attempted: 1,
    tokens_out: 0,
    latency_ms: 210,
    error_sentinel: "provider_quota",
    outcome: "failed",
    estimated_cost_microusd: 0,
  },
  {
    ...summary,
    id: "call-4",
    occurred_at: "2026-07-20T09:40:00Z",
    task: "owed_verdict",
    calls_attempted: 1,
    latency_ms: 1405,
    error_sentinel: "output_withheld",
    outcome: "declined",
    // No rate on the sheet for this model: the row says so rather than $0.
    estimated_cost_microusd: undefined,
  },
];

const detail = {
  ...summary,
  logical_call_id: "0b4e1f6a-6d7a-4c1e-9f0a-2b2a4f3c9d10",
  correlation_id: "6a9c2d1e-8b5f-4f1a-a2c3-7d8e9f0a1b2c",
  config_hash: "c0ffee12",
  served_identity_source: "response",
  context_scopes: ["identity"],
  context_fingerprint: "abc",
  attempts: [
    {
      attempt: 1,
      is_terminal: false,
      kind: "completion",
      attempt_reason: "",
      tier: "local_small",
      provider: "ollama",
      model_id: "llama-3.2",
      error_sentinel: "provider_error",
      tokens_in: 1840,
      tokens_out: 0,
      latency_ms: 400,
      occurred_at: summary.occurred_at,
    },
    {
      attempt: 2,
      is_terminal: true,
      kind: "completion",
      attempt_reason: "provider_error",
      tier: "cheap_cloud",
      provider: "gemini",
      model_id: "gemini-2.5-flash",
      tokens_in: 1840,
      tokens_out: 212,
      latency_ms: 900,
      occurred_at: summary.occurred_at,
    },
  ],
  payload_captured: true,
  payload: {
    request: {
      system: "Classify the captured email into one of the listed kinds.",
      messages: [{ role: "user", content: "Subject: Renewal quote for Q3" }],
    },
    response: '{"kind":"quote_request","confidence":0.91}',
  },
};

const brokered = [
  {
    ...summary,
    task: "capture_counterparty_verdict",
    tier: "premium",
    provider: "openai_compatible",
    served_model: "mistralai/mistral-small-2603",
    tokens_in: 1250344,
    tokens_out: 84213,
    latency_ms: 12873,
    decision_attempted: true,
    estimated_cost_microusd: 412_870,
  },
  {
    ...summary,
    id: "call-5",
    task: "stage_evidence_extract",
    tier: "cheap_cloud",
    provider: "openai_compatible",
    served_model: "google/gemini-3.1-flash-lite-preview-09-2026",
    tokens_in: 48211,
    tokens_out: 3120,
    latency_ms: 2210,
    outcome: "answered",
  },
];

function list(
  data: unknown[],
  options: Readonly<{
    capture?: boolean;
    allow?: GrantSpec;
    more?: boolean;
    callDetail?: unknown;
    /** Replaces the trace's answer: a read that never lands, or one that fails. */
    trace?: () => Response | Promise<Response>;
  }> = {},
) {
  const { capture = true, allow = OPERATOR, more = false } = options;
  return () => {
    // The dials live in the address, which the preview keeps between stories;
    // without this a search typed in one story would filter the next.
    globalThis.location.hash = "";
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow })),
      "GET /ai/calls":
        options.trace ??
        (() =>
          jsonResponse({
            data,
            page: more
              ? { has_more: true, next_cursor: "page-2" }
              : { has_more: false },
            payload_capture_enabled: capture,
            tasks: [
              "capture_classify",
              "enrich",
              "owed_verdict",
              "weekly_review",
            ],
            task_options: [
              {
                task: "capture_classify",
                display_name: "Message classification",
              },
              { task: "enrich" },
              {
                task: "owed_verdict",
                display_name: "Unanswered-message triage",
              },
              {
                task: "weekly_review",
                display_name: "Weekly review narrative",
              },
            ],
            models: ["claude-sonnet", "gemini-2.5-flash"],
          })),
      "GET /ai/calls/call-1": () => jsonResponse(options.callDetail ?? detail),
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

export const List: Story = { render: list(calls) };
export const ListDark: Story = {
  globals: { theme: "dark" },
  render: list(calls),
};
export const Empty: Story = { render: list([]) };
export const Withheld: Story = { render: list(calls, { allow: {} }) };
export const ListPaged: Story = { render: list(calls, { more: true }) };
export const Loading: Story = {
  render: list(calls, { trace: () => new Promise<Response>(() => {}) }),
};
export const Failed: Story = {
  render: list(calls, {
    trace: () => jsonResponse({ title: "Upstream is down", status: 500 }, 500),
  }),
};

// Reached from a figure that narrows the trace by vendor: the bar says what the
// cut covers and offers the way back.
export const FilteredByProvider: Story = {
  beforeEach: () => {
    const prior = globalThis.location.hash;
    globalThis.location.hash = "#/settings/model-calls?provider=openai";
    return () => {
      globalThis.location.hash = prior;
    };
  },
  render: () => {
    const view = list(calls)();
    globalThis.location.hash = "#/settings/model-calls?provider=openai";
    return view;
  },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: "Clear filters",
    });
  },
};

// The server found nothing for the search, so the answer is about the filters,
// not the installation, and the way back sits beside it.
export const FilteredEmpty: Story = {
  render: list([]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", { name: "Search calls" }),
      "no such call",
    );
    await canvas.findByText("No calls match these filters.");
  },
};

async function openFirstCall(canvasElement: HTMLElement, expected: RegExp) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: new RegExp(
        `Open the ${translate("en", "aiTask.name.capture_classify")} call`,
      ),
    }),
  );
  // The drawer is portalled out of the canvas, so it is found on the page.
  await within(canvasElement.ownerDocument.body).findByText(expected);
}

export const DrawerOpen: Story = {
  render: list(calls),
  play: ({ canvasElement }) => openFirstCall(canvasElement, /Response payload/),
};
export const DrawerOpenDark: Story = {
  ...DrawerOpen,
  globals: { theme: "dark" },
};

export const DrawerPayloadOff: Story = {
  render: list(calls, {
    capture: false,
    callDetail: { ...detail, payload_captured: false, payload: null },
  }),
  play: ({ canvasElement }) =>
    openFirstCall(canvasElement, /Payload capture is off/),
};

export const ListPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: list(calls),
};

// The settings column is 720 wide. Five columns and a broker's unbroken model
// ids still fit it, with no sideways scroll and every badge inside the card.
export const BrokerModelsAtSettingsWidth: Story = {
  render: () => {
    const Card = list(brokered);
    return (
      <div style={{ maxWidth: 720 }} data-testid="settings-width">
        <Card />
      </div>
    );
  },
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(/mistral-small-2603/);
    const scroller = canvasElement.querySelector(".table-scroll");
    await expect(scroller?.scrollWidth).toBeLessThanOrEqual(
      scroller?.clientWidth ?? 0,
    );
    const card = canvasElement.querySelector(".panel");
    const edge = card?.getBoundingClientRect().right ?? 0;
    for (const badge of canvasElement.querySelectorAll(".panel .badge")) {
      await expect(badge.getBoundingClientRect().right).toBeLessThanOrEqual(
        edge,
      );
    }
  },
};

export const BrokerModelsPhone: Story = {
  ...BrokerModelsAtSettingsWidth,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
