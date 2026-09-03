// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { WeeklyReview } from "./home.queries";
import { WeeklySection } from "./home.weekly";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The week that closed, on the Brief's weekly.
//
// It renders what was WRITTEN when the week closed — nothing here recomputes,
// because a retrospective that changed when you reopened it would not be one.
// So the frames differ in what the CLOSE recorded rather than in what the page
// decided: a week with a narrative and a week without it are two different
// records, and the second is what a run that could not read the account leaves
// behind.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const WEEK = "2026-06-29";
const PRIOR_WEEK = "2026-06-22";

const counts: WeeklyReview["counts"] = {
  tasks_due: 5,
  tasks_done: 4,
  tasks_carried_over: 2,
  deals_moved: 3,
  deals_won: 1,
  deals_lost: 1,
  proposals_accepted: 7,
  proposals_rejected: 2,
  brief_items_acted: 6,
  brief_items_dismissed: 3,
  leads_routed: 9,
  leads_answered_in_target: 7,
  leads_breached: 2,
  meetings_held: 5,
  meetings_with_next_step: 3,
  commitments_due: 4,
  commitments_kept: 3,
};

function review(over: Partial<WeeklyReview> = {}): WeeklyReview {
  return {
    id: "01a04000-0000-7000-8000-00000000000a",
    local_week_start: WEEK,
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    counts,
    deals: [
      {
        deal_id: "01a04000-0000-7000-8000-00000000000b",
        label: "Weber Rahmenvertrag",
        outcome: "won",
        occurred_at: "2026-07-02T14:00:00Z",
      },
      {
        deal_id: "01a04000-0000-7000-8000-00000000000c",
        label: "Nordwind Logistik renewal",
        outcome: "lost",
        occurred_at: "2026-07-03T09:30:00Z",
      },
    ],
    ...over,
  } as WeeklyReview;
}

function frame(latest: () => Response, weeks: readonly string[]) {
  const routes: RouteMap = {
    "GET /me": meRoute({}),
    "GET /weekly-reviews/latest": latest,
    "GET /weekly-reviews": () => jsonResponse({ weeks }),
  };
  installFetchStub(routes);
  return (
    <StoryProviders>
      <WeeklySection />
    </StoryProviders>
  );
}

const meta: Meta<typeof WeeklySection> = {
  title: "Shell/Weekly review",
  component: WeeklySection,
};
export default meta;

type Story = StoryObj<typeof WeeklySection>;

/** A closed week with everything the close could record: the outcomes as one
 *  strip, the workings under it, and the deals that ended named as they were
 *  called that week. */
export const Written: Story = {
  render: () =>
    frame(
      () =>
        jsonResponse(
          review({
            narrative: "Weber signed; two promises slipped to this week.",
            narrated_at: "2026-07-06T06:05:00Z",
          }),
        ),
      [WEEK],
    ),
};

/** A week whose readings exist and whose sentence does not. The absence is
 *  stated rather than papered over: an unwritten narrative is a fact about the
 *  run, and inventing one would put words in the account's mouth. */
export const WithoutNarrative: Story = {
  render: () =>
    frame(
      () => jsonResponse(review({ narrative: null, narrated_at: null })),
      [WEEK],
    ),
};

/** Two weeks on record, so the picker appears. Deltas hang off the prior week's
 *  own counts — a first week has nothing to have stayed level against and gets
 *  no delta line at all. */
export const AgainstThePriorWeek: Story = {
  render: () =>
    frame(
      () =>
        jsonResponse(
          review({
            prior: {
              local_week_start: PRIOR_WEEK,
              counts: { ...counts, deals_won: 3, commitments_kept: 1 },
            },
          } as Partial<WeeklyReview>),
        ),
      [WEEK, PRIOR_WEEK],
    ),
};

/** A rep whose first Monday has not come round yet. 404 is the honest answer;
 *  a page of zeroes would claim a week that was measured and empty. */
export const NoneYet: Story = {
  render: () => frame(() => jsonResponse({ title: "Not Found" }, 404), []),
};

/** A read the reader's grant refuses. The panel keeps its place and says so. */
export const Refused: Story = {
  render: () =>
    frame(
      () => jsonResponse({ title: "Forbidden", code: "forbidden" }, 403),
      [],
    ),
};
