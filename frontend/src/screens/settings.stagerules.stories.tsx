// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { StageRulesCard } from "./settings.stagerules";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What each transition is allowed to do. The states worth a picture are the
// three a reader cannot reach by clicking: a rule the PRODUCT stopped, which
// says so and offers the way back; the read that failed; and a viewer holding
// only the read, whose switches are refused with the reason.

type Policy = components["schemas"]["TransitionPolicy"];
type TransitionRecord = components["schemas"]["StageTransitionRecord"];

const PIPELINE_ID = "33333333-3333-4333-8333-333333333333";
const FROM = "44444444-4444-4444-8444-444444444444";
const TO = "55555555-5555-4555-8555-555555555555";
const WINDOW_DAYS = 30;

const transition: TransitionRecord = {
  pipeline_id: PIPELINE_ID,
  from_stage_id: FROM,
  to_stage_id: TO,
  from_stage_name: "Qualifying",
  to_stage_name: "Proposal",
  reviewed: 40,
  proposed: 2,
  expired: 1,
  superseded: 0,
  accepted_clean: 38,
  accepted_edited: 1,
  rejected: 1,
  auto_applied: 0,
  unsafe: 0,
  observation_days: 45,
  clean_acceptance_rate: 0.95,
  edit_rate: 0.02,
  rejection_rate: 0.02,
  unsafe_rate: 0,
  evidence_kinds: [],
};

function policy(over: Partial<Policy>): Policy {
  return {
    id: "66666666-6666-4666-8666-666666666666",
    pipeline_id: PIPELINE_ID,
    from_stage_id: FROM,
    to_stage_id: TO,
    mode: "auto",
    clean_acceptance_threshold: 0.9,
    correction_reversal_threshold: 0.05,
    min_reviewed: 20,
    min_observation_days: 30,
    window_days: WINDOW_DAYS,
    undo_window_hours: 24,
    version: 3,
    ...over,
  };
}

const meta: Meta<typeof StageRulesCard> = {
  title: "Settings/Sales/Stage automation/Transition rules",
  component: StageRulesCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageRulesCard>;

// The card reads the pipeline's rules, so each story answers that route rather
// than seeding a cache: data written with setQueryData is stale on arrival and
// the mount's background refetch would reach the network.
function Served({
  answer,
  mayChange = true,
  children,
}: Readonly<{
  answer: () => Response;
  mayChange?: boolean;
  children: ReactNode;
}>) {
  installFetchStub({
    "GET /me": meRoute(
      mayChange ? { pipeline: ["read", "update"] } : { pipeline: ["read"] },
    ),
    [`GET /stage-automation/policies/${PIPELINE_ID}`]: answer,
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/**
 * The product stopped this rule. The reason is drawn rather than summarised —
 * it is the whole basis on which somebody decides to start the transition
 * again — and the way back is the notice's own verb.
 */
export const Suspended: Story = {
  render: () => (
    <Served
      answer={() =>
        jsonResponse({
          data: [
            policy({
              suspended_at: "2026-08-14T09:12:00Z",
              suspended_reason:
                "Three of the last twelve moves were undone within a day.",
            }),
          ],
        })
      }
    >
      <StageRulesCard
        pipelineId={PIPELINE_ID}
        transitions={[transition]}
        reportWindowDays={WINDOW_DAYS}
      />
    </Served>
  ),
};

/** A transition running on its own, inside its undo window. */
export const Running: Story = {
  render: () => (
    <Served answer={() => jsonResponse({ data: [policy({})] })}>
      <StageRulesCard
        pipelineId={PIPELINE_ID}
        transitions={[transition]}
        reportWindowDays={WINDOW_DAYS}
      />
    </Served>
  ),
};

/**
 * A viewer who holds the read and not the update. The switches are drawn
 * refused with the reason rather than live and 403ing on the first press.
 */
export const ReadOnlyViewer: Story = {
  render: () => (
    <Served mayChange={false} answer={() => jsonResponse({ data: [] })}>
      <StageRulesCard
        pipelineId={PIPELINE_ID}
        transitions={[transition]}
        reportWindowDays={WINDOW_DAYS}
      />
    </Served>
  ),
};

/**
 * The rules did not load. The read's own failed face, which keeps the server's
 * account of it and offers the retry — the section has nothing else to say.
 */
export const Unreadable: Story = {
  render: () => (
    <Served
      answer={() =>
        jsonResponse(
          {
            title: "Internal server error",
            status: 500,
            detail: "The stage-automation store did not answer.",
          },
          500,
        )
      }
    >
      <StageRulesCard
        pipelineId={PIPELINE_ID}
        transitions={[transition]}
        reportWindowDays={WINDOW_DAYS}
      />
    </Served>
  ),
};
