// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ModelPriceDialog } from "./rate-manual";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// SETTING A VENDOR'S PRICES BY HAND, for a provider whose catalogue publishes
// none. The dialog is about that one vendor: the models the sheet already
// prices for it are listed to edit, and the model box offers what it serves.

const SHEET = [
  {
    provider: "gemini",
    model_id: "gemini-3.5-flash",
    lane: "chat",
    input_per_mtok: "0.3",
    output_per_mtok: "2.5",
    cache_read_per_mtok: "0.03",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  },
  {
    provider: "gemini",
    model_id: "gemini-3.1-flash-lite",
    lane: "chat",
    input_per_mtok: "0.1",
    output_per_mtok: "0.4",
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  },
];

function story(provider: string, refuse = false) {
  return () => {
    installFetchStub({
      "GET /ai-model-rates": () => jsonResponse({ data: SHEET }),
      "GET /ai/available-models/{provider}": () =>
        jsonResponse({ provider, models: [{ id: "gemini-4-pro" }] }),
      "POST /ai-model-rates": () =>
        refuse
          ? jsonResponse(
              {
                type: "https://errors.gradion.com/validation",
                title: "Unprocessable",
                status: 422,
                code: "rate_past",
                detail: "effective_date cannot be in the past",
              },
              422,
            )
          : jsonResponse({ ...SHEET[0], input_per_mtok: "0.35" }, 201),
    });
    return (
      <StoryProviders>
        <ModelPriceDialog provider={provider} onClose={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ModelPriceDialog> = {
  title: "Settings/AI/Models and routing/Set prices by hand",
  component: ModelPriceDialog,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ModelPriceDialog>;

/** The vendor's priced models, with the form empty beneath them. */
export const ForAProvider: Story = { render: story("gemini") };
export const ForAProviderDark: Story = {
  render: story("gemini"),
  globals: { theme: "dark" },
};

/** A model loaded from the list, ready to re-price. */
export const EditingAnExistingPrice: Story = {
  render: story("gemini"),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Edit gemini-3.5-flash" }),
    );
  },
};

/** The server refused the write; the form stays as typed. */
export const Refused: Story = {
  render: story("gemini", true),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Edit gemini-3.5-flash" }),
    );
    await userEvent.click(await body.findByRole("button", { name: "Save" }));
  },
};

/** At 390px the dialog is a full-screen sheet and the price table scrolls inside it. */
export const ForAProviderPhone: Story = {
  render: story("gemini"),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
