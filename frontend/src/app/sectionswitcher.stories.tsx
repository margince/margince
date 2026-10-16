// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../i18n/en";
import type { Route } from "../lib/router";
import { SETTINGS_SCREEN, useSettingsSection } from "../screens/settingsnav";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { sectionHead } from "./pagemeta";
import { SectionSwitcher } from "./sectionswitcher";
import "./shell.css";

// The phone page head's switcher, fed the settings level a seat with pipeline
// and tag writes resolves.
function Switcher({ route }: Readonly<{ route: Route }>) {
  const head = sectionHead(useSettingsSection(route), route);
  return head ? (
    <SectionSwitcher section={head.section} entry={head.entry} />
  ) : null;
}

function render({ route }: Readonly<{ route: Route }>) {
  installFetchStub({
    "GET /me": meRoute(
      {
        pipeline: ["read", "create", "update"],
        tag: ["read", "create", "update"],
      },
      { roles: ["manager"], seat: "full" },
    ),
  });
  return (
    <StoryProviders>
      <Switcher route={route} />
    </StoryProviders>
  );
}

const meta = {
  title: "Shell/Section switcher",
  component: Switcher,
  args: { route: { screen: SETTINGS_SCREEN, id: "tags" } },
  render,
} satisfies Meta<typeof Switcher>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The sheet lands on the reader's own page, not on its search field, so no
// keyboard rises over the list.
export const Open: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: en["shell.sectionSwitch"].replace(
          "{name}",
          en["settings.tab.tags"],
        ),
      }),
    );
    const sheet = within(canvasElement.ownerDocument.body);
    const current = await sheet.findByRole("link", {
      name: en["settings.tab.tags"],
      current: "page",
    });
    await expect(current).toHaveFocus();
  },
};

export const OpenDark: Story = {
  ...Open,
  globals: { viewport: { value: "phone" }, theme: "dark" },
  tags: ["uat-phone"],
};
