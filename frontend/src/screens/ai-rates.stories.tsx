// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ModelRatePlate } from "./ai-rates";

// The three states worth looking at are the three answers the price sheet can
// give: it knows both models, it knows one, it knows neither. The last is the
// one that matters — a reader who typed an id the sheet has never seen must not
// be shown a price, and must not be shown a zero.
const meta: Meta<typeof ModelRatePlate> = {
  title: "Onboarding/Model price plate",
  component: ModelRatePlate,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ModelRatePlate>;

const SHEET = [
  {
    provider: "gemini",
    model_id: "gemini-2.5-flash",
    lane: "chat" as const,
    input_per_mtok: "0.30",
    output_per_mtok: "2.50",
    cache_read_per_mtok: "0.075",
    cache_write_per_mtok: "0.3833",
    effective_date: "2026-08-01",
  },
  {
    provider: "gemini",
    model_id: "text-embedding-004",
    lane: "embeddings" as const,
    input_per_mtok: "0.15",
    output_per_mtok: "0",
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  },
];

// Both lanes priced: the row read across is what one call costs here.
export const BothPriced: Story = {
  args: {
    catalogue: SHEET,
    provider: "gemini",
    chatModel: "gemini-2.5-flash",
    embedModel: "text-embedding-004",
    locale: "en",
  },
};

// A chat id the sheet has never seen. The binding is legitimate — the server
// accepts any id the vendor serves — so the slot says what that costs the
// reader later rather than refusing the choice.
export const ChatModelNotOnSheet: Story = {
  args: {
    catalogue: SHEET,
    provider: "gemini",
    chatModel: "gemini-3.0-preview",
    embedModel: "text-embedding-004",
    locale: "en",
  },
};

// Nothing priced, which is what an installation whose sheet failed to load
// looks like. Two warned slots, no figure invented for either.
export const NothingPriced: Story = {
  args: {
    catalogue: [],
    provider: "openrouter",
    chatModel: "anthropic/claude-sonnet-4.5",
    embedModel: "openai/text-embedding-3-small",
    locale: "en",
  },
};
