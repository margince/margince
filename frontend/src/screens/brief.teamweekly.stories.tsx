// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { team, teamWeek } from "./brief.fixtures";
import { TeamWeeklyPanel } from "./brief.teamweekly";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// A team's frozen week, with the picker that chooses which team it is about.
//
// The frames here are the three shapes the PAGE has, which is a different
// question from what one panel draws: before a team is chosen, after, and the
// week where nothing moved. The first of those was a blank page under a
// dropdown until it was given a body — a surface a reader cannot tell from one
// that failed to load.
//
// EVERY INSTANT IS FIXED, like the rep's weekly beside it: a review built from
// `new Date()` documents whichever day the catalog was opened on.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

/** Two teams, so the picker is a real choice, plus whichever week is asked for. */
function page(week: () => Response): RouteMap {
  return {
    "GET /teams": () =>
      jsonResponse({
        data: [team, { id: "t-sued", name: "Süd" }],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /weekly-reviews/team": week,
  };
}

function panel(routes: RouteMap) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <TeamWeeklyPanel offered />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof TeamWeeklyPanel> = {
  title: "Shell/Brief team weekly",
  component: TeamWeeklyPanel,
};
export default meta;

type Story = StoryObj<typeof TeamWeeklyPanel>;

// Nothing chosen yet. The picker is capped to a reading width and the page
// under it says what it is waiting for — blank, the same page read as a
// surface whose content had failed to arrive.
export const NoTeamChosen: Story = {
  render: panel(page(() => jsonResponse(teamWeek))),
};

// The same wait in the dark theme, where an empty page and a failed one are
// hardest of all to tell apart.
export const NoTeamChosenDark: Story = {
  globals: { theme: "dark" },
  render: panel(page(() => jsonResponse(teamWeek))),
};

// One team, read straight through: a control whose only option is the one
// already showing asks the reader to confirm what they cannot change. The
// week's readings, its movement rows and the agenda are the whole page.
export const OneTeamReadStraight: Story = {
  render: panel({
    "GET /teams": () =>
      jsonResponse({
        data: [team],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /weekly-reviews/team": () => jsonResponse(teamWeek),
  }),
};

// The same week in the dark theme.
export const OneTeamReadStraightDark: Story = {
  globals: { theme: "dark" },
  render: panel({
    "GET /teams": () =>
      jsonResponse({
        data: [team],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /weekly-reviews/team": () => jsonResponse(teamWeek),
  }),
};

// A week in which nothing moved. The strip still reports the zeros as figures,
// because a zero is a count; the movement rows are GONE, because a bar has no
// baseline to be drawn against and five empty tracks read as a reading that
// failed to load rather than as a quiet week.
export const NothingMoved: Story = {
  render: panel({
    "GET /teams": () =>
      jsonResponse({
        data: [team],
        page: { next_cursor: null, has_more: false },
      }),
    "GET /weekly-reviews/team": () =>
      jsonResponse({
        ...teamWeek,
        counts: {
          ...teamWeek.counts,
          deals_won: 0,
          deals_lost: 0,
          deals_moved: 0,
          meetings_held: 0,
          leads_routed: 0,
        },
      }),
  }),
};
