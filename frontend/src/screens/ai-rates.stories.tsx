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

// The vendor's live catalogue, as it is actually shaped: chat models only, so
// only the chat lane can ever be answered from it.
const VENDOR = {
  rankedBy: "Artificial Analysis intelligence index",
  unavailable: false,
  models: [
    {
      model_id: "anthropic/claude-opus-5",
      name: "Anthropic: Claude Opus 5",
      input_per_mtok: "5",
      output_per_mtok: "25",
      rank_score: "63.1",
    },
  ],
};

// THE ONE TO LOOK AT. The chat id is not on the sheet, but the vendor is
// serving it and published a price, so the slot shows that price marked as the
// vendor's rather than warning. The embed lane beside it is the same screen's
// other half and stays warned, because no vendor with a public catalogue
// publishes embedding models and a figure there would be invented.
//
// The two slots side by side ARE the provenance rule: indigo says a machine
// went and read this, and the unmarked slot beside it would be a number this
// installation had agreed to. Worth flipping the Theme control, since the whole
// distinction is carried by `--aiText`.
export const ProposedByTheVendor: Story = {
  args: {
    catalogue: [],
    vendor: VENDOR,
    provider: "openai_compatible",
    chatModel: "anthropic/claude-opus-5",
    embedModel: "openai/text-embedding-3-small",
    locale: "en",
  },
};

// The sheet wins wherever it has a row. Same vendor list, but this chat model
// is priced on the sheet, so the recorded price is what shows and the vendor's
// asking price is not offered as an alternative: two prices for one model is a
// question the reader cannot answer.
export const RecordedOutranksProposed: Story = {
  args: {
    catalogue: SHEET,
    vendor: {
      ...VENDOR,
      models: [{ ...VENDOR.models[0], model_id: "gemini-2.5-flash" }],
    },
    provider: "gemini",
    chatModel: "gemini-2.5-flash",
    embedModel: "text-embedding-004",
    locale: "en",
  },
};
