// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { WorklistBody } from "./worklist.body";
import { row } from "./worklist.fixtures";
import type { Worklist } from "./worklist.queries";

const day: Worklist = {
  as_of: "2026-08-31T09:00:00Z",
  scope: "mine",
  scope_options: ["mine"],
  summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
  sources_unavailable: [],
  reach: [],
  counts: [],
  queue: [],
  readings: {
    changed_since_brief: 0,
    revenue_at_risk_minor: null,
    buyer_replies: 0,
    prospecting: 0,
    review: 0,
    more_available: false,
  },
};
const meta: Meta<typeof WorklistBody> = {
  title: "Records/Worklist/Body",
  component: WorklistBody,
  decorators: [
    (Story, context) => {
      stubWithSession(
        {
          "GET /worklist/handled": () =>
            jsonResponse({ receipts: [], truncated: false }),
        },
        {},
      );
      return (
        <StoryProviders
          locale={
            context.parameters.locale === "de"
              ? "de"
              : context.parameters.locale === "vi"
                ? "vi"
                : "en"
          }
        >
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: {
    day,
    queue: [],
    scope: "mine",
    view: "today",
    filter: "all",
    owner: "",
    selectedId: "none",
    onScope: fn(),
    onView: fn(),
    onFilter: fn(),
    onOwner: fn(),
    onSelect: fn(),
    onOpenEmail: fn(),
    onRefresh: fn(),
    onMore: fn(),
    cursor: undefined,
    loadingMore: false,
    moreFailed: false,
  },
};
export default meta;
type Story = StoryObj<typeof WorklistBody>;
export const EmptyPageWithContinuation: Story = {
  args: { cursor: "page-2", loadingMore: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("Nothing in this batch yet. More loads as you scroll."),
    ).toBeVisible();
    await expect(canvas.getByText("Loading more…")).toBeVisible();
  },
};
export const EmptyPageDark: Story = {
  ...EmptyPageWithContinuation,
  globals: { theme: "dark" },
};
// The Decisions view: its rows in one panel named by the view, and the line at
// its end once every batch is loaded.
export const DecisionsView: Story = {
  args: {
    view: "decisions",
    queue: [
      row({
        source: "dedupe_candidate",
        category: "decisions",
        destination: "review",
      }),
    ],
    day: {
      ...day,
      counts: [
        {
          category: "decisions",
          considered: 1,
          shown: 1,
          more_available: false,
        },
      ],
      summary: { ...day.summary, total: 1, lower_priority: 1 },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("All decisions loaded."),
    ).toBeVisible();
  },
};
export const DecisionsViewDark: Story = {
  ...DecisionsView,
  globals: { theme: "dark" },
};

// A failed next batch keeps the loaded rows and offers Retry at the end.
export const NextBatchFailed: Story = {
  args: {
    ...DecisionsView.args,
    cursor: "page-2",
    moreFailed: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Could not load more.")).toBeVisible();
    await expect(canvas.getByRole("button", { name: "Retry" })).toBeVisible();
  },
};
export const NextBatchFailedDark: Story = {
  ...NextBatchFailed,
  globals: { theme: "dark" },
};

// The System view with nothing in it says which kind of work is clear.
export const SystemViewClear: Story = {
  args: { view: "system" },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("No unresolved issues."),
    ).toBeVisible();
  },
};

// Today ends with the Upcoming and backlog section and its count.
export const TodayWithUpcoming: Story = {
  args: {
    queue: [row({ title: "Send the pricing sheet" })],
    day: {
      ...day,
      counts: [
        { category: "tasks", considered: 1, shown: 1, more_available: false },
        {
          category: "upcoming",
          considered: 4,
          shown: 0,
          more_available: false,
        },
      ],
      summary: { ...day.summary, total: 1, due: 1 },
    },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("Upcoming and backlog"),
    ).toBeVisible();
  },
};
export const TodayWithUpcomingDark: Story = {
  ...TodayWithUpcoming,
  globals: { theme: "dark" },
};

export const EmptyPageGermanDark: Story = {
  ...EmptyPageDark,
  parameters: { locale: "de" },
  play: undefined,
};
export const EmptyPageVietnamese: Story = {
  ...EmptyPageWithContinuation,
  parameters: { locale: "vi" },
  play: undefined,
};

export const OptionalReconnections: Story = {
  args: {
    day: {
      ...day,
      optional_relationships: [
        row({
          id: "01930000-0000-7000-8000-000000000031",
          source: "relationship_decay",
          category: "relationships",
          level: 6,
          title: "Demo contact",
          because: [{ kind: "quiet_days", value: { kind: "days", days: 180 } }],
          actions: ["open", "dismiss"],
          move: { action: "draft_email" },
        }),
      ],
      optional_relationships_more_available: true,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Optional reconnections")).toBeVisible();
    await expect(canvas.getByText("Demo contact")).toBeVisible();
    await expect(
      canvas.getByText("More optional contacts are outside this view."),
    ).toBeVisible();
  },
};
export const OptionalReconnectionsDark: Story = {
  ...OptionalReconnections,
  globals: { theme: "dark" },
};
