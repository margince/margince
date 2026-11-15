// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { MeetingNextStep } from "./meetingnextstep";

const MEETING_ID = "01a05500-0000-7000-8000-0000000000b2";

const meta: Meta<typeof MeetingNextStep> = {
  title: "Records/Meeting next step",
  component: MeetingNextStep,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MeetingNextStep>;

/** Asked right after a meeting is marked held: a next step to file, or Skip. */
export const AskedAfterHeld: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { activity: ["read", "create"] } })),
      "POST /activities": () => jsonResponse({ id: "task-1", kind: "task" }),
    });
    return (
      <StoryProviders>
        <MeetingNextStep
          meetingId={MEETING_ID}
          invalidateKeys={[]}
          onClose={() => undefined}
        />
      </StoryProviders>
    );
  },
};
