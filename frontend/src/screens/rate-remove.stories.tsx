// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RemovePriceDialog } from "./rate-remove";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The question asked before a model's price entry leaves the sheet: every date
// of it goes, and the calls it priced read as unpriced from then on.

const row = {
  provider: "jev_compatible",
  model_id: "~typesafe/jev-latest",
  lane: "decisions" as const,
  input_per_mtok: "0.01",
  output_per_mtok: "0.01",
  cache_read_per_mtok: "0",
  cache_write_per_mtok: "0",
  effective_date: "2026-09-29",
  source: "seed" as const,
};

function story(refuse = false) {
  return () => {
    installFetchStub({
      "DELETE /ai-model-rates": () =>
        refuse
          ? jsonResponse(
              {
                type: "https://errors.gradion.com/forbidden",
                title: "Forbidden",
                status: 403,
                code: "permission_denied",
                detail: "Removing a price needs the rates grant.",
              },
              403,
            )
          : new Response(null, { status: 204 }),
    });
    return (
      <StoryProviders>
        <RemovePriceDialog
          row={row}
          onClose={() => {}}
          returnFocusTo={() => null}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof RemovePriceDialog> = {
  title: "Settings/AI/AI models/Remove a price",
  component: RemovePriceDialog,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof RemovePriceDialog>;

export const Default: Story = { render: story() };
export const Dark: Story = { render: story(), globals: { theme: "dark" } };
