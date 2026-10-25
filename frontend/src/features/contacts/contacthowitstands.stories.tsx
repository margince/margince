// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { ContactHowItStands } from "./contacthowitstands";
import { populated } from "./contactpage.fixtures";
import "./contact360.css";

const meta = {
  title: "Records/Contact 360/How it stands",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;
type Contact360 = components["schemas"]["Contact360"];

function render(view: Contact360) {
  return () => (
    <StoryProviders>
      <ContactHowItStands view={view} />
    </StoryProviders>
  );
}

export const Standing: Story = { render: render(populated) };

// A visit logged by its day alone, on the day the page was read. It has
// begun by the morning read, and its mark names the day without a clock.
const visitedToday: Contact360 = {
  ...populated,
  activities: {
    ...populated.activities,
    data: [
      ...(populated.activities?.data ?? []),
      {
        id: "a-visit",
        kind: "note",
        subject: "Site visit",
        occurred_at: "2026-08-13T12:00:00Z",
        occurred_precision: "day",
        source: "manual",
        captured_by: "human:u1",
        created_at: "2026-08-13T15:00:00Z",
        updated_at: "2026-08-13T15:00:00Z",
        is_done: true,
      },
    ],
    page: { has_more: false, next_cursor: null },
  },
};

export const DayOnlyVisit: Story = {
  render: render(visitedToday),
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: en["timeline.kind.note"] }),
    );
    const row = (await screen.findByText("Site visit")).parentElement;
    await expect(row?.textContent).toMatch(/13 Aug/);
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};
