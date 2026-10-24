// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import { SETTINGS_PAGES } from "../../lib/settingscatalog";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SettingsSearchBox } from "./settingssearchbox";
import "../../app/shell.css";

// The installation's settings, which Configuration answers to by name.
const SETTING_NAMES = [
  { key: "http.port", env: "MARGINCE_HTTP_PORT" },
  { key: "worker.concurrency", env: "MARGINCE_WORKER_CONCURRENCY" },
];

// The box the settings rail and the phone overview carry, over every page.
function Box() {
  installFetchStub({
    "GET /me": meRoute({}, { roles: ["admin"], seat: "full" }),
    "GET /installation/configuration": () => jsonResponse(SETTING_NAMES),
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

// A setting's own name finds the page that draws it, and the hit names it.
export const SectionHit: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("combobox", {
        name: en["settings.search.label"],
      }),
      "data retention",
    );
    const hit = await canvas.findByRole("option", {
      name: new RegExp(en["settings.tab.system-health"]),
    });
    await expect(hit).toHaveTextContent(en["operations.retention.label"]);
  },
};

export const SectionHitDark: Story = {
  ...SectionHit,
  globals: { theme: "dark" },
};

// A setting's variable finds Configuration, which opens filtered to it.
export const SettingNameHit: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("combobox", {
        name: en["settings.search.label"],
      }),
      "MARGINCE_HTTP_PORT",
    );
    const hit = await canvas.findByRole("option", {
      name: new RegExp(en["settings.tab.configuration"]),
    });
    await expect(hit).toHaveTextContent("http.port");
    await expect(hit).toHaveAttribute(
      "href",
      "#/settings/configuration?q=MARGINCE_HTTP_PORT",
    );
  },
};

export const SettingNameHitDark: Story = {
  ...SettingNameHit,
  globals: { theme: "dark" },
};

export const SectionHitPhone: Story = {
  ...SectionHit,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
