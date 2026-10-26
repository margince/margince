// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen } from "storybook/test";
import type { components } from "../../api/schema";
import { useLocale, useT } from "../../i18n";
import { en } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import { company360 } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { NextMeetingStat } from "./companynextmeeting";

const meta = {
  title: "Records/Company 360/Next meeting",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;
type Company360 = components["schemas"]["Company360"];

function Stat({ view }: Readonly<{ view: Company360 }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <NextMeetingStat
      view={view}
      locale={locale}
      recordZone={useRecordZone()}
      t={t}
    />
  );
}

function render(view: Company360) {
  return () => (
    <StoryProviders>
      <div style={{ maxWidth: "16rem" }}>
        <Stat view={view} />
      </div>
    </StoryProviders>
  );
}

export const Booked: Story = {
  render: render({
    ...company360,
    as_of: "2026-08-13T09:00:00Z",
    next_meeting: {
      activity_id: "m-walk",
      starts_at: "2026-08-20T10:00:00Z",
      occurred_precision: "instant",
      subject: "Walkthrough",
      participants: [],
    },
  }),
};

// Read in the afternoon, a meeting known only by its day is still today's.
// Its stored noon is past, yet the strip must not call the account empty.
export const DayOnlyToday: Story = {
  render: render({
    ...company360,
    as_of: "2026-08-13T15:00:00Z",
    next_meeting: {
      activity_id: "m-walk",
      starts_at: "2026-08-13T12:00:00Z",
      occurred_precision: "day",
      subject: "Walkthrough",
      participants: [],
    },
  }),
  play: async () => {
    await screen.findByText("Walkthrough");
    await expect(
      screen.queryByText(en["co.strip.next.none"]),
    ).not.toBeInTheDocument();
  },
};
