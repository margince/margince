// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PlanSection } from "./brief.plan";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
import type { WeeklyPlan, WeeklyPlanCommitment } from "./weeklyplan.queries";

// The week ahead, on the Brief's weekly.
//
// Four frames, because the panel's states differ in what the reader is being
// asked for rather than in how much data arrived: a week nobody has opened is
// an INVITATION, a week opened and empty is an absence, a week with rows is the
// work, and a refused read is the panel talking about itself. The assembled
// page can only ever show one.
//
// Read every frame in BOTH themes with the toolbar's Theme control.

const WEEK_START = "2026-06-08";

function commitment(
  over: Partial<WeeklyPlanCommitment> = {},
): WeeklyPlanCommitment {
  return {
    id: "c1",
    label: "Call the Aster buyer back",
    state: "open",
    position: 1,
    due_on: null,
    help_requested: null,
    manager_response: null,
    manager_user_id: null,
    responded_at: null,
    completed_at: null,
    ...over,
  } as WeeklyPlanCommitment;
}

function plan(over: Partial<WeeklyPlan> = {}): WeeklyPlan {
  return {
    id: "p1",
    local_week_start: WEEK_START,
    status: "open",
    commitments: [commitment()],
    ...over,
  } as WeeklyPlan;
}

/** The panel's own read, plus the session probe every screen makes. */
function routes(current: () => Response): RouteMap {
  return {
    "GET /me": meRoute({}),
    "GET /weekly-plans/current": current,
  };
}

function frame(current: () => Response) {
  installFetchStub(routes(current));
  return (
    <StoryProviders>
      <PlanSection />
    </StoryProviders>
  );
}

const meta: Meta<typeof PlanSection> = {
  title: "Shell/Weekly plan",
  component: PlanSection,
};
export default meta;

type Story = StoryObj<typeof PlanSection>;

/** A rep's Monday. 404 is not a failure here — it is a week nobody has opened,
 *  so the panel offers rather than reporting one. */
export const NotStarted: Story = {
  render: () => frame(() => jsonResponse({ title: "Not Found" }, 404)),
};

/** Opened and still bare. The sentence is the same one the section's own empty
 *  arm says, in the same voice — a card's answer to "there is none" is card
 *  body text wherever it is drawn. */
export const Empty: Story = {
  render: () => frame(() => jsonResponse(plan({ commitments: [] }))),
};

/** The week as it is worked: an open row, one settled, one the close marked as
 *  missed, and one the rep has asked their lead for help on. */
export const Planned: Story = {
  render: () =>
    frame(() =>
      jsonResponse(
        plan({
          commitments: [
            commitment({ due_on: "2026-06-11" }),
            commitment({
              id: "c2",
              label: "Send the Weber quote",
              state: "done",
              position: 2,
              completed_at: "2026-06-09T09:12:00Z",
            }),
            commitment({
              id: "c3",
              label: "Reopen the Nordwind renewal",
              state: "missed",
              position: 3,
            }),
            commitment({
              id: "c4",
              label: "Agree the Globex close date",
              position: 4,
              help_requested: "I cannot reach their buyer — can you introduce?",
            }),
          ],
        }),
      ),
    ),
};

/** A read the reader's grant refuses. The panel keeps its place and says so,
 *  which is what stops a denial reading as a week with nothing in it. */
export const Refused: Story = {
  render: () =>
    frame(() => jsonResponse({ title: "Forbidden", code: "forbidden" }, 403)),
};
