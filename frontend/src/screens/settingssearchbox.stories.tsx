// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { SETTINGS_PAGES } from "./settingscatalog";
import { SettingsSearchBox } from "./settingssearchbox";
import "../app/shell.css";

// The box the settings rail and the phone overview carry, over every page.
function Box() {
  installFetchStub({
    "GET /me": meRoute({}, { roles: ["admin"], seat: "full" }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: "18rem" }}>
        <SettingsSearchBox pages={SETTINGS_PAGES} />
      </div>
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Settings search box",
  component: Box,
} satisfies Meta<typeof Box>;
export default meta;
type Story = StoryObj<typeof meta>;

// Nothing typed offers nothing: the rail beside it already lists every page.
export const EmptyQuery: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const box = await canvas.findByRole("combobox", {
      name: en["settings.search.label"],
    });
    await expect(box).toHaveAttribute("aria-expanded", "false");
    await expect(canvas.queryByRole("listbox")).toBeNull();
  },
};

// Each hit leads with its page's glyph, as the rail row does.
export const GlyphHit: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("combobox", {
        name: en["settings.search.label"],
      }),
      "audit",
    );
    const hit = await canvas.findByRole("option", {
      name: new RegExp(en["settings.tab.audit"]),
    });
    await expect(hit.querySelector("svg")).not.toBeNull();
  },
};

export const GlyphHitDark: Story = {
  ...GlyphHit,
  globals: { theme: "dark" },
};

// The first Escape clears the text and keeps the reader in the box.
export const TypedThenEscape: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const box = await canvas.findByRole("combobox", {
      name: en["settings.search.label"],
    });
    await userEvent.type(box, "tag");
    await canvas.findByRole("listbox");
    await userEvent.keyboard("{Escape}");
    await expect(box).toHaveValue("");
    await expect(box).toHaveFocus();
    await expect(canvas.queryByRole("listbox")).toBeNull();
  },
};
