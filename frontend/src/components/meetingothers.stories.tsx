// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../testing/story-utils";
import { MeetingOthers } from "./meetingothers";

// What a meeting card names after its contacts: colleagues, addresses with no
// contact, and how many more attendees there are than the eight it names.

const meta: Meta = {
  title: "Records/Meetings/Meeting others",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** After two contacts: a colleague, a lead's address, and four more. */
export const AfterContacts: Story = {
  render: () => (
    <StoryProviders>
      <p className="t-caption">
        Ines Duarte, Jonas Weber
        <MeetingOthers
          others={[
            { kind: "colleague", name: "Sam Okafor" },
            { kind: "address", name: "Lena Prospect" },
          ]}
          more={4}
          afterContacts
        />
      </p>
    </StoryProviders>
  ),
};
