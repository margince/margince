// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { RoutineDeliveryRows } from "./routinedelivery";

const meta = {
  title: "Settings/You/Notifications/Routine email choices",
  component: RoutineDeliveryRows,
  parameters: { layout: "padded" },
} satisfies Meta<typeof RoutineDeliveryRows>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Personal: Story = {
  args: { settings: {}, save: () => {} },
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /teams": () => jsonResponse({ data: [] }),
    });
    return (
      <StoryProviders>
        <Panel>
          <SettingList>
            <RoutineDeliveryRows {...args} />
          </SettingList>
        </Panel>
      </StoryProviders>
    );
  },
};
export const Phone: Story = {
  ...Personal,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
