// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SettingsSavePage } from "./settingssavepage";
import Configuration from "./systemhealthconfiguration";

const meta = {
  title: "Settings/Governance/System health/Configuration",
  component: Configuration,
} satisfies Meta<typeof Configuration>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Schedules: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["read", "update"] }),
      "GET /installation/settings": () =>
        jsonResponse({ operations: defaultOperations }),
    });
    return (
      <StoryProviders>
        <SettingsSavePage>
          <Configuration />
        </SettingsSavePage>
      </StoryProviders>
    );
  },
};
export const Dark: Story = { ...Schedules, globals: { theme: "dark" } };
