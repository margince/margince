// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  DisplayNameSettingRow,
  GreetingNameSettingRow,
} from "./settings-names";

function NameRows() {
  const toast = useToast();
  return (
    <Panel>
      <SettingList bleed="settings">
        <DisplayNameSettingRow toast={toast} />
        <GreetingNameSettingRow toast={toast} />
      </SettingList>
    </Panel>
  );
}

function render() {
  installFetchStub({ "GET /me": () => jsonResponse(meFixture({})) });
  return (
    <StoryProviders>
      <NameRows />
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

// Settled: each row is its field alone, with no Save waiting.
export const Settled: Story = { render };

// Edited: Save appears under the field that moved, and only there.
export const Edited: Story = {
  render,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const field = await canvas.findByRole("textbox", { name: "Greeting name" });
    await userEvent.type(field, "Ada");
  },
};

export const EditedPhone: Story = {
  ...Edited,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const EditedDark: Story = { ...Edited, globals: { theme: "dark" } };
