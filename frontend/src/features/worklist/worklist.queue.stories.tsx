// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import { ReviewPanel } from "./worklist.queue";

const meta: Meta<typeof ReviewPanel> = {
  title: "Records/Worklist/Review totals",
  component: ReviewPanel,
  decorators: [
    (Story, context) => {
      stubWithSession({}, {});
      return (
        <StoryProviders
          locale={
            context.parameters.locale === "de"
              ? "de"
              : context.parameters.locale === "vi"
                ? "vi"
                : "en"
          }
        >
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    items: [
      row({
        source: "dedupe_candidate",
        category: "decisions",
        destination: "review",
      }),
    ],
    shortfall: { loaded: 1, total: 3, floor: false },
    more: false,
    rows: {
      positions: new Map(),
      owner: "",
      selectedId: "",
      onSelect: fn(),
      onOpenEmail: fn(),
      onFilter: fn(),
      picks: { offered: false, picked: new Set(), toggle: fn() },
    },
  },
};
export default meta;
type Story = StoryObj<typeof ReviewPanel>;
export const Departures: Story = {
  args: { changed: true },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(/1 of 3 snapshot items shown/),
    ).toBeVisible();
  },
};
export const DeparturesDark: Story = {
  ...Departures,
  globals: { theme: "dark" },
};
export const MorePages: Story = { args: { more: true } };
export const Finished: Story = {};
export const Bounded: Story = {
  args: { shortfall: { loaded: 1, total: 1, floor: true } },
};
export const FilteredWithMore: Story = {
  args: { shortfall: null, more: true },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(/More pages may contain review items/),
    ).toBeVisible();
  },
};

export const DeparturesGermanDark: Story = {
  ...DeparturesDark,
  parameters: { locale: "de" },
  play: undefined,
};
export const DeparturesVietnamese: Story = {
  ...Departures,
  parameters: { locale: "vi" },
  play: undefined,
};
