// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { SaveFilterListAction } from "./filterlist";
import { listsMe, liveList, TEAM_ID, teamsPage } from "./lists.fixtures";
import { ListSettingsAction } from "./listsettings";
import { ListAudienceFields } from "./listsharing";
import { newGroup, newLeaf } from "./segmentpredicate";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Who can find a list, asked where a list is made and where it is changed. A
// team audience also asks which team: one of the reader's, or all of them.
const meta: Meta = { title: "Patterns/List sharing" };
export default meta;

type Story = StoryObj;

function routes() {
  installFetchStub({
    "GET /me": listsMe(true, [TEAM_ID]),
    "GET /teams": () => jsonResponse(teamsPage),
  });
}

export const TeamAudience: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <ListAudienceFields
          value={{ sharing: "team", teamId: TEAM_ID }}
          onChange={() => undefined}
          ownerIsReader
        />
      </StoryProviders>
    );
  },
};

export const SaveAsLiveList: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <SaveFilterListAction
          resource="company"
          tree={newGroup("and", [newLeaf("industry", "eq", "Manufacturing")])}
        />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Save as Live List" }),
    );
  },
};

export const EditList: Story = {
  render: () => {
    routes();
    return (
      <StoryProviders>
        <ListSettingsAction list={{ ...liveList, team_id: TEAM_ID }} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit list" }),
    );
  },
};
