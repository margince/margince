// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import type { Worklist } from "./worklist.queries";
import { DayUnread } from "./worklist.unread";

// What a day did not read, said by the surface about itself. An unread source
// and an unread teammate's plan are the same claim and share one notice; a team
// read that covered every plan says so quietly, because that line is what tells
// a lead an empty week is empty rather than unasked.
//
// The last frame PINS the dark theme: a frame nobody flips is a frame the
// render gate draws in one theme only. The others take the toolbar's control.

const teammates = ["Lena Fischer", "Marc Weber", "Sofia Ruiz", "Ana Novak"];

function aTeamDay(unread: readonly string[], truncated = false): Worklist {
  return {
    as_of: "2026-08-31T09:00:00Z",
    scope: "team",
    scope_options: ["mine", "unassigned", "team", "all"],
    summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    sources_unavailable: [],
    reach: [],
    readings: {
      changed_since_brief: 0,
      revenue_at_risk_minor: null,
      buyer_replies: 0,
      prospecting: 0,
      review: 0,
      more_available: false,
    },
    counts: [],
    queue: [],
    plan_coverage: {
      members: teammates.map((display_name, at) => ({
        user_id: `00000000-0000-4000-8000-00000000000${at + 1}`,
        display_name,
        read: !unread.includes(display_name),
      })),
      truncated,
    },
  };
}

const meta: Meta<typeof DayUnread> = {
  title: "Records/Worklist/Day unread",
  component: DayUnread,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DayUnread>;

/** Every teammate's plan was read: one quiet line, no warning. */
export const EveryPlanRead: Story = {
  render: () => (
    <StoryProviders>
      <DayUnread day={aTeamDay([])} />
    </StoryProviders>
  ),
};

/** One plan unread, named, inside the notice an unread source would raise. */
export const APlanUnread: Story = {
  render: () => (
    <StoryProviders>
      <DayUnread day={aTeamDay(["Ana Novak"])} />
    </StoryProviders>
  ),
};

/** An unread source and an unread plan in the one notice, with the team list
 *  cut short so later teammates were never asked. */
export const ASourceAndAPlanUnreadCutShort: Story = {
  render: () => (
    <StoryProviders>
      <DayUnread
        day={{
          ...aTeamDay(["Marc Weber"], true),
          sources_unavailable: [{ source: "task", reason: "failed" }],
        }}
      />
    </StoryProviders>
  ),
};

/** The fullest notice in the dark theme, where the callout's warning tone sits
 *  on a different ground. */
export const ASourceAndAPlanUnreadDark: Story = {
  ...ASourceAndAPlanUnreadCutShort,
  globals: { theme: "dark" },
};

export const BoundedPersonalAndTeamSources: Story = {
  render: (_args, context) => {
    const day = aTeamDay([]);
    day.reach = [
      {
        source: "task",
        considered: 12,
        shown: 12,
        more_available: true,
        personal: false,
      },
      {
        source: "notice",
        considered: 30,
        shown: 30,
        more_available: true,
        personal: true,
      },
    ];
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
        <DayUnread day={day} />
      </StoryProviders>
    );
  },
};

export const BoundedDark: Story = {
  ...BoundedPersonalAndTeamSources,
  globals: { theme: "dark" },
};
export const BoundedGermanDark: Story = {
  ...BoundedDark,
  parameters: { locale: "de" },
};
export const BoundedVietnamese: Story = {
  ...BoundedPersonalAndTeamSources,
  parameters: { locale: "vi" },
};
