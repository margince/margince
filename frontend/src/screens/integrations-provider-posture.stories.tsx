// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { SettingList } from "../design-system/settingrow";
import { LookupPostureRow } from "./integrations-provider-posture";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The installation's automatic-lookup switch on its own, in the states the
// provider card draws it in: writable, read-only, and a posture that could not
// be read.

function postureStory(canEdit: boolean, answer: () => Response) {
  return () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({ allow: { integrations: ["read", "update"] } }),
        ),
      "GET /integrations/settings": answer,
    });
    return (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <SettingList>
            <LookupPostureRow canEdit={canEdit} />
          </SettingList>
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LookupPostureRow> = {
  title: "Settings/Data/Integrations/Automatic lookup",
  component: LookupPostureRow,
};
export default meta;
type Story = StoryObj<typeof LookupPostureRow>;

export const On: Story = {
  render: postureStory(true, () => jsonResponse({ automatic_lookup: true })),
};

export const ReadOnly: Story = {
  render: postureStory(false, () => jsonResponse({ automatic_lookup: false })),
};

export const ReadFailed: Story = {
  render: postureStory(true, () =>
    jsonResponse({ code: "internal", detail: "unavailable" }, 500),
  ),
};

export const OnDark: Story = {
  globals: { theme: "dark" },
  render: postureStory(true, () => jsonResponse({ automatic_lookup: true })),
};
