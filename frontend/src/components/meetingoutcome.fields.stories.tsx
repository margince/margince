// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { type OutcomeDraft, OutcomeEditFields } from "./meetingoutcome.fields";

const meta: Meta<typeof OutcomeEditFields> = {
  title: "Patterns/Meeting outcome fields",
  component: OutcomeEditFields,
};
export default meta;
type Story = StoryObj<typeof OutcomeEditFields>;

function OutcomeFieldsStory({
  managed = false,
}: Readonly<{ managed?: boolean }>) {
  const [draft, setDraft] = useState<OutcomeDraft>({
    status: "held",
    subject: "Pricing review",
    body: "Send the revised quote after the review.",
    day: "2026-07-06",
  });
  return (
    <StoryProviders>
      <div className="form-stack">
        <OutcomeEditFields
          draft={draft}
          managed={managed}
          transcript={false}
          days={{ latest: "2026-07-06", today: "2026-07-06" }}
          onChange={setDraft}
          onDateChange={() => {}}
        >
          {null}
        </OutcomeEditFields>
      </div>
    </StoryProviders>
  );
}

export const CapturedMeeting: Story = {
  render() {
    installFetchStub({ "GET /me": meRoute({ activity: ["read", "update"] }) });
    return <OutcomeFieldsStory />;
  },
};
export const CapturedMeetingDark: Story = {
  ...CapturedMeeting,
  globals: { theme: "dark" },
};
export const CalendarManaged: Story = {
  render() {
    installFetchStub({ "GET /me": meRoute({ activity: ["read", "update"] }) });
    return <OutcomeFieldsStory managed />;
  },
};
export const CalendarManagedDark: Story = {
  ...CalendarManaged,
  globals: { theme: "dark" },
};
