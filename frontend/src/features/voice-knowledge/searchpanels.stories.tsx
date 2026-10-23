// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen } from "storybook/test";
import { SEARCH_ACTIVITY_SEGMENT } from "../../lib/activityroute";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { useSearchPanels } from "./searchpanels";

function OriginalMeeting() {
  const { drawers } = useSearchPanels("source", SEARCH_ACTIVITY_SEGMENT);
  return <>{drawers}</>;
}

const meta = {
  title: "Records/Record 360/Source route",
  component: OriginalMeeting,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
} satisfies Meta<typeof OriginalMeeting>;
export default meta;

export const Meeting: StoryObj<typeof meta> = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/source": () =>
        jsonResponse({
          id: "source",
          kind: "meeting",
          subject: "Project review",
          body: "Morgan will send the revised schedule.",
          occurred_at: "2026-10-20T08:30:00Z",
          content_state: "available",
          links: [],
          meeting_participants: [
            { contact_id: "buyer", full_name: "Dana Buyer" },
          ],
        }),
    });
    return <OriginalMeeting />;
  },
  play: async () => {
    await screen.findByText("Morgan will send the revised schedule.");
    await screen.findByRole("link", { name: "Dana Buyer" });
  },
};
