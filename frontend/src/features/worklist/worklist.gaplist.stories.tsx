// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { jsonResponse, StoryProviders } from "../../testing/story-utils";
import { CloseTheGap } from "./worklist.gaplist";

// The deals a team should work to meet its quarter goal, as the daily freeze
// ranked them. Each row shows the stored facts behind its rank. A masked
// amount shows no figure. Read every frame in both themes.

type ForecastFocus = components["schemas"]["ForecastFocus"];
type FocusTeam = ForecastFocus["teams"][number];

const TEAM = "00000000-0000-4000-8000-0000000000a1";
const LENA = "00000000-0000-4000-8000-000000000001";

function team(over: Partial<FocusTeam>): ForecastFocus {
  return {
    period_start: "2026-10-01",
    period_end: "2026-12-31",
    teams: [
      {
        team_id: TEAM,
        snapshot_id: "00000000-0000-4000-8000-0000000000b1",
        taken_at: "2026-11-18T01:00:00Z",
        base_currency: "EUR",
        status: "behind",
        deals: [],
        ...over,
      },
    ],
    withheld: false,
  };
}

const ranked: FocusTeam["deals"] = [
  {
    rank: 1,
    deal_id: "00000000-0000-4000-8000-0000000000d1",
    deal_name: "Northstar renewal",
    owner_id: LENA,
    weighted_minor: 42_000_00,
    reason: {
      closes_in_days: -4,
      close_provisional: false,
      signals: [{ code: "close_pushed", count: 3, close_passed: true }],
    },
  },
  {
    rank: 2,
    deal_id: "00000000-0000-4000-8000-0000000000d2",
    deal_name: "Hafen Logistik expansion",
    owner_id: LENA,
    weighted_minor: 31_500_00,
    reason: {
      closes_in_days: 3,
      close_provisional: true,
      signals: [{ code: "no_next_step", closes_in_days: 3 }],
    },
  },
  {
    rank: 3,
    deal_id: "00000000-0000-4000-8000-0000000000d3",
    deal_name: "Masked pilot",
    owner_id: LENA,
    reason: { closes_in_days: 21, close_provisional: false, signals: [] },
  },
];

function frame(body: ForecastFocus) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/forecast/focus")) return jsonResponse(body);
    if (url.includes("/teams")) {
      return jsonResponse({
        data: [{ id: TEAM, name: "Field sales" }],
        page: {},
      });
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
  return (
    <StoryProviders>
      <CloseTheGap />
    </StoryProviders>
  );
}

const meta: Meta<typeof CloseTheGap> = {
  title: "Records/Worklist/Close the gap",
  component: CloseTheGap,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CloseTheGap>;

/** Behind the goal: the deals by weighted amount, raised for close-date risk. */
export const BehindTheGoal: Story = {
  render: () => frame(team({ deals: ranked })),
};

/** On target: only the deals whose close dates put the goal at risk. */
export const OnTarget: Story = {
  render: () => frame(team({ status: "on_target", deals: ranked.slice(0, 2) })),
};

/** On target with nothing at risk says so in one line. */
export const NothingAtRisk: Story = {
  render: () => frame(team({ status: "on_target" })),
};

/** A team with a month of the quarter unplanned has no list, and says why. */
export const NoGoal: Story = {
  render: () => frame(team({ status: "no_goal" })),
};

/** Individual performance is off and the only team would name a colleague. */
export const HeldBackBySwitch: Story = {
  render: () => frame({ ...team({}), teams: [], withheld: true }),
};
