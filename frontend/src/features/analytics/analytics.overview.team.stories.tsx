// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  metric,
  teamEvaluation,
  teamHandoffs,
  teamReview,
  upcomingWindow,
} from "./analytics.overview.fixtures";
import { waitingHandoffs } from "./analytics.overview.scenariodata";
import {
  ReasonsDrawer,
  WaitingDrawer,
} from "./analytics.overview.team.drawers";
import { MemberTable } from "./analytics.overview.team.members";
import { TeamStanding } from "./analytics.overview.team.standing";

// Team prospecting's own parts, each in the states it keeps apart. The whole
// page in every scenario is in the Overview stories.
const meta: Meta = { title: "Records/Reports/Analytics/Team prospecting" };
export default meta;
type Story = StoryObj;

type Summary = components["schemas"]["SdrSummary"];
const ZONE = "Europe/Berlin";
const review = teamReview();

function summary(): Readonly<{ data: Summary; isError: boolean }> {
  const data: Summary = {
    as_of: review.as_of,
    scope: review.scope,
    workflow: review.workflow,
    upcoming_meetings: 4,
    upcoming_window: upcomingWindow(review.as_of),
  };
  return { data, isError: false };
}

const qualification = teamReview({
  workflow: "qualification",
  handoffs: undefined,
  member_targets: {
    metric: "leads_promoted",
    window: {
      start_at: "2026-08-31T22:00:00Z",
      end_at: "2026-09-30T22:00:00Z",
    },
  },
  members: review.members.map((member, index) => ({
    ...member,
    ...(index === 0 ? { target: { value: 8, actual: 5 } } : {}),
  })),
});

const qualificationEvaluation = () => {
  const base = teamEvaluation();
  return {
    ...base,
    metrics: base.metrics.map((reading) =>
      reading.id === "leads_promoted"
        ? metric("leads_promoted", 5, { denominator: 12 })
        : reading,
    ),
  };
};

const returns = teamReview({
  handoffs: teamHandoffs({
    outcomes: { submitted: 6, accepted: 3, rejected: 1, recycled: 3 },
    reasons: [
      { outcome: "recycled", label: "", given: false, count: 2 },
      {
        outcome: "recycled",
        label: "Needs more qualification",
        given: true,
        count: 1,
      },
      { outcome: "rejected", label: "Wrong territory", given: true, count: 1 },
    ],
  }),
});

const noop = () => undefined;

// A handoff team: the cohort with its rate, then what waits, what came back
// and the leads still owed a reply.
export const HandoffStanding: Story = {
  render: () => (
    <StoryProviders>
      <TeamStanding
        review={review}
        readings={{ data: teamEvaluation(), isError: false }}
        summary={summary()}
        scope={{ kind: "team", id: review.scope.id }}
        zone={ZONE}
        onEvidence={noop}
      />
    </StoryProviders>
  ),
};

// Three rounds is below the floor, so the counts show and no rate does.
export const TooFewRounds: Story = {
  render: () => (
    <StoryProviders>
      <TeamStanding
        review={teamReview({
          handoffs: teamHandoffs({
            cohort: {
              rounds: 3,
              accepted: 1,
              rejected: 0,
              recycled: 1,
              waiting: 1,
            },
          }),
        })}
        readings={{ data: teamEvaluation(), isError: false }}
        summary={summary()}
        scope={{ kind: "team", id: review.scope.id }}
        zone={ZONE}
        onEvidence={noop}
      />
    </StoryProviders>
  ),
};

export const QualificationStanding: Story = {
  render: () => (
    <StoryProviders>
      <TeamStanding
        review={qualification}
        readings={{ data: qualificationEvaluation(), isError: false }}
        summary={summary()}
        scope={{ kind: "team", id: review.scope.id }}
        zone={ZONE}
        onEvidence={noop}
      />
    </StoryProviders>
  ),
};

// The readings are still being read, so the panel names that and never says
// nothing waits.
export const ReadingsPending: Story = {
  render: () => (
    <StoryProviders>
      <TeamStanding
        review={qualification}
        readings={{ isError: false }}
        summary={summary()}
        scope={{ kind: "team", id: review.scope.id }}
        zone={ZONE}
        onEvidence={noop}
      />
    </StoryProviders>
  ),
};

export const HandoffMembers: Story = {
  render: () => (
    <StoryProviders>
      <MemberTable review={review} zone={ZONE} />
    </StoryProviders>
  ),
};

// Qualified first, then the member's own monthly target where one is set.
export const QualificationMembers: Story = {
  render: () => (
    <StoryProviders>
      <MemberTable review={qualification} zone={ZONE} />
    </StoryProviders>
  ),
};

// Each figure the reader's role leaves out is named under the table, never a
// column of zeros.
export const HiddenFigures: Story = {
  render: () => (
    <StoryProviders>
      <MemberTable
        review={{
          ...qualification,
          withheld_figures: ["meetings", "leads", "targets"],
          member_targets: undefined,
          members: qualification.members.map(
            ({ meetings_held: _m, qualified: _q, unanswered: _u, ...row }) =>
              row,
          ),
        }}
        zone={ZONE}
      />
    </StoryProviders>
  ),
};

export const IndividualPerformanceOff: Story = {
  render: () => (
    <StoryProviders>
      <MemberTable
        review={teamReview({ members_withheld: true, members: [] })}
        zone={ZONE}
      />
    </StoryProviders>
  ),
};

// The returns of the period by reason, a decision with no reason included.
export const ReturnReasons: Story = {
  render: () => (
    <StoryProviders>
      <ReasonsDrawer
        outcome="recycled"
        review={returns}
        handoffs={teamHandoffs(returns.handoffs)}
        zone={ZONE}
        onClose={noop}
      />
    </StoryProviders>
  ),
};

export const WaitingHandoffs: Story = {
  render: () => {
    installFetchStub({
      "GET /analytics/sdr-summary/handoffs": () =>
        jsonResponse({
          as_of: review.as_of,
          rows: waitingHandoffs(review.as_of),
        }),
    });
    return (
      <StoryProviders>
        <WaitingDrawer
          scope={{ kind: "team", id: review.scope.id }}
          review={review}
          onClose={noop}
        />
      </StoryProviders>
    );
  },
};

export const HandoffStandingDark: Story = {
  ...HandoffStanding,
  globals: { theme: "dark" },
};

export const QualificationMembersOnAPhone: Story = {
  ...QualificationMembers,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
