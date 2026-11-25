// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import { meFixture } from "../../testing/mefixture";
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
      "GET /teams": () => jsonResponse({ data: [], page: { has_more: false } }),
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

const BERLIN = { id: "00000000-0000-4000-8000-000000000001", name: "Berlin" };

/** A reviewer with a team chosen: "No team" heads the list, so the choice can
 * be undone. */
export const TeamChosen: Story = {
  args: {
    settings: { routine_mail_scope: "team", routine_mail_team_id: BERLIN.id },
    save: () => {},
  },
  render: (args) => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({ ...meFixture(), team_week: "every_team", teams: [] }),
      "GET /teams": () =>
        jsonResponse({ data: [BERLIN], page: { has_more: false } }),
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
