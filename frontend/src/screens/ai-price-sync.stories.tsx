// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { ModelPricesCard } from "./ai-price-sync";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The Model prices card: where prices come from, whether they sync daily, and
// what the last sync did for each vendor that had something to say.

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

function stub(
  allow: GrantSpec,
  state: { auto_sync: boolean; last_run?: unknown },
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
  });
}

function Demo() {
  return (
    <StoryProviders>
      <ModelPricesCard />
    </StoryProviders>
  );
}

const meta: Meta<typeof Demo> = {
  title: "Settings/AI/AI models/Model prices",
  component: Demo,
};
export default meta;
type Story = StoryObj<typeof Demo>;

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
