// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ModelPricesCard } from "./ai-price-sync";

// The Model prices card: whether prices sync daily and what the last sync did.
// Under that, the prices in force, one fold per vendor.

const ADMIN: GrantSpec = { ai_model_rate: ["read", "create", "update"] };

const line = (
  provider: string,
  outcome: string,
  counts: Partial<
    Record<"updated" | "unchanged" | "added" | "kept", number>
  > = {},
) => ({
  provider,
  outcome,
  updated: 0,
  unchanged: 0,
  added: 0,
  kept: 0,
  models: [],
  unlisted: [],
  ...counts,
});

const LAST_RUN = {
  ran_at: new Date(Date.now() - 2 * 3_600_000).toISOString(),
  trigger: "scheduled",
  report: {
    providers: [
      line("gemini", "updated", { updated: 3, added: 2 }),
      line("anthropic", "unchanged", { unchanged: 4, kept: 1 }),
      line("openai", "not_configured"),
      line("ollama", "not_available"),
    ],
  },
};

const RATES = {
  data: [
    {
      provider: "anthropic",
      model_id: "claude-opus-4-8",
      input_per_mtok: "5",
      output_per_mtok: "25",
      cache_read_per_mtok: "0.5",
      cache_write_per_mtok: "6.25",
      effective_date: "2026-07-23",
    },
    {
      provider: "gemini",
      model_id: "gemini-3.5-flash",
      input_per_mtok: "1.5",
      output_per_mtok: "9",
      cache_read_per_mtok: "0.15",
      cache_write_per_mtok: "0",
      effective_date: "2026-07-23",
    },
  ],
};

function stub(
  allow: GrantSpec,
  state: { auto_sync: boolean; last_run?: unknown },
  rates: unknown = RATES,
) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow })),
    "GET /ai/price-sync": () => jsonResponse(state),
    "PUT /ai/price-sync": (body) =>
      jsonResponse(
        typeof body === "object" && body !== null
          ? { ...state, ...body }
          : state,
      ),
    "POST /ai-model-rates/refresh": () => jsonResponse(LAST_RUN.report),
    "GET /ai-model-rates": () => jsonResponse(rates),
  });
}

function Demo() {
  return (
    <StoryProviders>
      <ModelPricesCard />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/AI/AI models/Model prices",
  component: Demo,
} satisfies Meta<typeof Demo>;
export default meta;
type Story = StoryObj<typeof meta>;

export const NeverSynced: Story = {
  render: () => {
    stub(ADMIN, { auto_sync: true });
    return <Demo />;
  },
};
export const Synced: Story = {
  render: () => {
    stub(ADMIN, { auto_sync: true, last_run: LAST_RUN });
    return <Demo />;
  },
};
export const AutoSyncOff: Story = {
  render: () => {
    stub(ADMIN, { auto_sync: false, last_run: LAST_RUN });
    return <Demo />;
  },
};
export const Reader: Story = {
  render: () => {
    stub({ ai_model_rate: ["read"] }, { auto_sync: true, last_run: LAST_RUN });
    return <Demo />;
  },
};
export const SyncedDark: Story = {
  ...Synced,
  globals: { theme: "dark" },
};
export const NoPricesYet: Story = {
  render: () => {
    stub(ADMIN, { auto_sync: true }, { data: [] });
    return <Demo />;
  },
};
// The auto-sync switch and the verbs in the head at 390px.
export const SyncedPhone: Story = {
  ...Synced,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
// One vendor's fold opened: its prices in force.
export const VendorOpen: Story = {
  ...Synced,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText("Anthropic"));
    await canvas.findByText("claude-opus-4-8");
  },
};
