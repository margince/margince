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
    filter: "all",
    owner: "",
    selectedId: "none",
    onScope: fn(),
    onFilter: fn(),
    onOwner: fn(),
    onSelect: fn(),
    onOpenEmail: fn(),
    onRefresh: fn(),
    onMore: fn(),
    hasMore: false,
    loadingMore: false,
    moreFailed: false,
  },
};
export default meta;
type Story = StoryObj<typeof WorklistBody>;
export const EmptyPageWithContinuation: Story = {
  args: { hasMore: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("Nothing on this page. Load more to continue."),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: /load more/i }),
    ).toBeVisible();
  },
};
export const EmptyPageDark: Story = {
  ...EmptyPageWithContinuation,
  globals: { theme: "dark" },
};
export const ChangedReviewTotals: Story = {
  args: {
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
          considered: 3,
          shown: 1,
          more_available: false,
        },
      ],
      readings: { ...day.readings, review: 3 },
      summary: {
        ...day.summary,
        total: 3,
        lower_priority: 3,
        buckets: { urgent: 0, due_today: 0, planned: 0, review: 3 },
      },
    },
    walk: { as_of: day.as_of, new_available: 0, changed_since_snapshot: 2 },
  },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText(/1 of 3 snapshot items shown/),
    ).toBeVisible();
  },
};
export const ChangedReviewDark: Story = {
  ...ChangedReviewTotals,
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
