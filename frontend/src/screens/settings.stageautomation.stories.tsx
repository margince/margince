// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { StageAutomationCard } from "./settings.stageautomation";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// What each stage transition has earned, and what each is allowed to do. Three
// states are told apart on purpose: a report with rows, a pipeline nothing has
// been proposed on, and a read that failed — drawing one set of words for all
// three would tell somebody to wait for numbers that were refused.

type TransitionRecord = components["schemas"]["StageTransitionRecord"];

const PIPELINE_ID = "77777777-7777-4777-8777-777777777777";
const WINDOW_DAYS = 30;

const pipelines = {
  data: [
    { id: PIPELINE_ID, name: "New business", is_default: true, position: 0 },
  ],
};

const transition: TransitionRecord = {
  pipeline_id: PIPELINE_ID,
  from_stage_id: "88888888-8888-4888-8888-888888888888",
  to_stage_id: "99999999-9999-4999-8999-999999999999",
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

const meta: Meta<typeof StageAutomationCard> = {
  title: "Settings/Sales/Stage automation/Transition record",
  component: StageAutomationCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StageAutomationCard>;

// Both reads are answered rather than seeded into a cache: data written with
// setQueryData is stale on arrival and the mount's background refetch would
// reach the network.
function Served({
  routes,
  children,
}: Readonly<{ routes: RouteMap; children: ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({ pipeline: ["read", "update"] }),
    "GET /pipelines": () => jsonResponse(pipelines),
    ...routes,
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/** One transition with a record worth reading, and its rules under the table. */
export const WithRecord: Story = {
  render: () => (
    <Served
      routes={{
        "GET /stage-automation/report": () =>
          jsonResponse({ data: [transition], window_days: WINDOW_DAYS }),
        [`GET /stage-automation/policies/${PIPELINE_ID}`]: () =>
          jsonResponse({ data: [] }),
      }}
    >
      <StageAutomationCard />
    </Served>
  ),
};

/** Nothing has been proposed on this pipeline yet, which is a fact a reader
 * acts on by waiting — so it says that and not "no data". */
export const NothingProposedYet: Story = {
  render: () => (
    <Served
      routes={{
        "GET /stage-automation/report": () =>
          jsonResponse({ data: [], window_days: WINDOW_DAYS }),
      }}
    >
      <StageAutomationCard />
    </Served>
  ),
};

/** The report was refused. It says so rather than drawing an empty table,
 * which a reader would take for "nothing has happened". */
export const ReportUnreadable: Story = {
  render: () => (
    <Served
      routes={{
        "GET /stage-automation/report": () =>
          jsonResponse(
            {
              title: "Internal server error",
              status: 500,
              detail: "The report could not be assembled.",
            },
            500,
          ),
      }}
    >
      <StageAutomationCard />
    </Served>
  ),
};
