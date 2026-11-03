// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { currentParams } from "../lib/urlstate";
import { StoryProviders } from "../testing/story-utils";
import { RefusedFiltersNote } from "./refusedfiltersnote";

// A list's note for filters the reader's role refuses: it names them, and its
// one press takes them out of the address.
const meta: Meta<typeof RefusedFiltersNote> = {
  title: "Patterns/Refused filters note",
  component: RefusedFiltersNote,
  parameters: { layout: "padded" },
  beforeEach: () => {
    globalThis.history.replaceState(
      null,
      "",
      "#/leads?industry=software&country=DE&status=new",
    );
    return () => globalThis.history.replaceState(null, "", "#/");
  },
};
export default meta;

type Story = StoryObj<typeof RefusedFiltersNote>;

export const TwoFilters: Story = {
  render: () => (
    <StoryProviders>
      <RefusedFiltersNote
        message="Your role cannot narrow leads by Industry, Country."
        dials={["industry", "country"]}
        names="Industry, Country"
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", {
        name: "Remove Industry, Country",
      }),
    );
    await expect([...currentParams().keys()]).toEqual(["status"]);
  },
};
