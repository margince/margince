// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { NO_PICKS } from "./worklist.bulkdone";
import { row } from "./worklist.fixtures";
import type { Worklist, WorklistItem } from "./worklist.queries";
import type { RowContext } from "./worklist.queue";
import { UpcomingSection } from "./worklist.upcoming";
import "./worklist.css";

// Upcoming and backlog, at the end of Today: tasks due after today, read only
// once the reader opens the section, with its own exact count.

function day(queue: WorklistItem[], upcoming: number): Worklist {
  return {
    as_of: "2026-09-02T09:00:00Z",
    scope: "mine",
    scope_options: ["mine"],
    view: "today",
    queue,
    summary: { urgent: 0, due: 0, in_play: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    counts: [
      {
        category: "upcoming",
        considered: upcoming,
        shown: queue.length,
        more_available: false,
      },
    ],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: 0,
      revenue_currency: "EUR",
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
  };
}

const later = [
  row({
    id: "u1",
    category: "upcoming",
    title: "Prepare the renewal offer",
    due_at: "2026-09-04T09:00:00Z",
  }),
  row({
    id: "u2",
    category: "upcoming",
    title: "Check in after the pilot",
    due_at: "2026-09-09T09:00:00Z",
  }),
];

const rows: RowContext = {
  positions: new Map(),
  owner: "",
  selectedId: "",
  onSelect: () => {},
  onOpenEmail: () => {},
  onView: () => {},
  picks: NO_PICKS,
};

function frame(upcoming: WorklistItem[]) {
  return () => {
    installFetchStub({
      "GET /worklist": () => jsonResponse(day(upcoming, upcoming.length)),
    });
    return (
      <StoryProviders>
        <UpcomingSection
          day={day([], upcoming.length)}
          scope="mine"
          owner=""
          rows={rows}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof UpcomingSection> = {
  title: "Records/Worklist/Upcoming",
  component: UpcomingSection,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof UpcomingSection>;

export const Opened: Story = {
  render: frame(later),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText(en["worklist.filter.upcoming"]));
    await canvas.findByText("Prepare the renewal offer");
    await canvas.findByText(en["worklist.upcoming.end"]);
  },
};

export const NothingPlanned: Story = {
  render: frame([]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText(en["worklist.filter.upcoming"]));
    await canvas.findByText(en["worklist.upcoming.empty"]);
  },
};
