// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { ModelPriceDialog } from "./rate-manual";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The free-form "add a rate": the provider is chosen from the vendors the
// product knows, and the model box then offers what that vendor serves. A
// vendor's own prices are edited in its sheet, through the same form.

function story() {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({ allow: { ai_model_rate: ["read", "create", "update"] } }),
        ),
      "GET /ai-model-rates": () => jsonResponse({ data: [] }),
      "GET /ai/available-models/{provider}": () =>
        jsonResponse({ provider: "gemini", models: [{ id: "gemini-4-pro" }] }),
    });
    return (
      <StoryProviders>
        <ModelPriceDialog onClose={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ModelPriceDialog> = {
  title: "Settings/AI/AI usage/Add a model rate",
  component: ModelPriceDialog,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof ModelPriceDialog>;

export const Default: Story = { render: story() };
export const Dark: Story = { render: story(), globals: { theme: "dark" } };
