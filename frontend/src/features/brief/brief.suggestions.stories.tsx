// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import { ranked } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { BriefSuggestions } from "./brief.suggestions";

const meta: Meta<typeof BriefSuggestions> = {
  title: "Shell/Home/Stored suggestions",
  component: BriefSuggestions,
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /brief": () =>
          jsonResponse({
            ...ranked,
            items: [
              {
                ...ranked.items[0],
                finding: "Buyer asked to revisit rollout in November.",
              },
            ],
          }),
        "GET /deals/d-1": () =>
          jsonResponse({ id: "d-1", name: "Acme rollout" }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof BriefSuggestions>;
export const StoredRun: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText(en["brief.suggestions.review"]));
    await expect(
      await canvas.findByRole("link", { name: "Acme rollout" }),
    ).toBeVisible();
    await expect(
      canvas.getByText(/Buyer asked to revisit rollout/),
    ).toBeVisible();
  },
};
export const StoredRunDark: Story = {
  ...StoredRun,
  globals: { theme: "dark" },
};
export const StoredRunPhone: Story = { ...StoredRun, tags: ["uat-phone"] };

// A run no overnight pass has annotated: the ranked deals stay reachable as a
// plain list under the line that says no assessment was written yet.
export const NotAnnotated: Story = {
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /brief": () =>
          jsonResponse({
            ...ranked,
            annotated_at: null,
            items: [{ ...ranked.items[0], finding: null }],
          }),
        "GET /deals/d-1": () =>
          jsonResponse({ id: "d-1", name: "Acme rollout" }),
      });
      return <Story />;
    },
  ],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText(en["brief.suggestions.review"]));
    await expect(
      await canvas.findByText(en["brief.suggestions.notAnnotated"]),
    ).toBeVisible();
    await expect(
      await canvas.findByRole("link", { name: "Acme rollout" }),
    ).toBeVisible();
  },
};
