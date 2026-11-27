// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
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
    complete: false,
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
      canvas.getByText(englishCopy["worklist.emptyPage"]),
    ).toBeVisible();
    await expect(
      canvas.getByText(englishCopy["scrollSentinel.loading"]),
    ).toBeVisible();
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
    complete: true,
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(englishCopy["worklist.end.decisions"]),
    ).toBeVisible();
  },
};
export const DecisionsViewDark: Story = {
  ...DecisionsView,
  globals: { theme: "dark" },
};

// No cursor, but the last page did not say the list is whole: a source held
// rows back. So the end makes no "all loaded" claim over the rows shown.
export const DecisionsViewNotWhole: Story = {
  args: { ...DecisionsView.args, complete: false },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getAllByRole("listitem").length).toBeGreaterThan(0);
    await expect(
      canvas.queryByText(englishCopy["worklist.end.decisions"]),
    ).toBeNull();
  },
};

// A failed next batch keeps the loaded rows and offers Retry at the end.
export const NextBatchFailed: Story = {
  args: {
    ...DecisionsView.args,
    cursor: "page-2",
    complete: false,
    moreFailed: true,
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(englishCopy["scrollSentinel.failed"]),
    ).toBeVisible();
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
      within(canvasElement).getByText(englishCopy["worklist.clear.system"]),
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
      within(canvasElement).getByText(englishCopy["worklist.filter.upcoming"]),
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
    await expect(
      canvas.getByText(englishCopy["worklist.relationships.optional"]),
    ).toBeVisible();
    await expect(
      canvas.getByText("No recent contact with Demo contact"),
    ).toBeVisible();
    await expect(
      canvas.getByText(englishCopy["worklist.relationships.more"]),
    ).toBeVisible();
  },
};
export const OptionalReconnectionsDark: Story = {
  ...OptionalReconnections,
  globals: { theme: "dark" },
};
