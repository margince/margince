// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { SettingList } from "../../design-system/settingrow";
import { en as englishCopy } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { NameSettingRows, useAccountNames } from "./settings-names";

function NameRows() {
  const names = useAccountNames();
  return (
    <Panel>
      <SettingList bleed="settings">
        <NameSettingRows names={names} />
      </SettingList>
    </Panel>
  );
}

function render() {
  installFetchStub({ "GET /me": () => jsonResponse(meFixture({})) });
  return (
    <StoryProviders>
      <SavePageProvider>
        <NameRows />
        <PageSaveBar />
      </SavePageProvider>
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/You/Account/Name rows",
  component: NameRows,
  parameters: { layout: "padded" },
} satisfies Meta<typeof NameRows>;
export default meta;
type Story = StoryObj<typeof meta>;

// Settled: each row is its field alone, and the page shows no save bar.
export const Settled: Story = { render };

// Edited: the page's save bar appears once a name moves.
export const Edited: Story = {
  render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = await canvas.findByRole("textbox", {
      name: englishCopy["settings.greetingName"],
    });
    await userEvent.type(field, "Ada");
  },
};

export const EditedPhone: Story = {
  ...Edited,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const EditedDark: Story = { ...Edited, globals: { theme: "dark" } };
