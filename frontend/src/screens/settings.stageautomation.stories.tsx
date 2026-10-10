// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { userEvent, within } from "storybook/test";
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

// A pipeline with a track record. One transition is trusted and one has no
// answers yet. The last names an evidence kind this build has no label for.
const populated: TransitionRecord[] = [
  {
    ...transition,
    evidence_kinds: [
      { kind: "document_signed", reviewed: 14, accepted_clean: 11, unsafe: 0 },
      { kind: "buyer_confirmed", reviewed: 26, accepted_clean: 25, unsafe: 0 },
    ],
  },
  {
    ...transition,
    from_stage_id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
    from_stage_name: "Proposal",
    to_stage_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    to_stage_name: "Negotiation",
    reviewed: 0,
    proposed: 3,
    expired: 1,
    accepted_clean: 0,
    accepted_edited: 0,
    rejected: 0,
    observation_days: 0,
    clean_acceptance_rate: 0,
    edit_rate: 0,
    rejection_rate: 0,
  },
  {
    ...transition,
    from_stage_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    from_stage_name: "Negotiation",
    to_stage_id: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
    to_stage_name: "Closed won",
    reviewed: 250,
    proposed: 0,
    unsafe: 1,
    observation_days: 21,
    clean_acceptance_rate: 0.79,
    edit_rate: 0.1,
    rejection_rate: 0.1,
    unsafe_rate: 0.004,
    evidence_kinds: [
      { kind: "terms_accepted", reviewed: 200, accepted_clean: 160, unsafe: 1 },
      { kind: "po_received", reviewed: 50, accepted_clean: 38, unsafe: 0 },
    ],
  },
];

const populatedRoutes: RouteMap = {
  "GET /stage-automation/report": () =>
    jsonResponse({ data: populated, window_days: WINDOW_DAYS }),
  [`GET /stage-automation/policies/${PIPELINE_ID}`]: () =>
    jsonResponse({
      automation_enabled: true,
      data: [
        {
          id: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
          pipeline_id: PIPELINE_ID,
          from_stage_id: populated[0].from_stage_id,
          to_stage_id: populated[0].to_stage_id,
          mode: "auto",
          clean_acceptance_threshold: 0.9,
          correction_reversal_threshold: 0.01,
          min_reviewed: 20,
          min_observation_days: 28,
          window_days: WINDOW_DAYS,
          undo_window_hours: 72,
          version: 2,
        },
      ],
    }),
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

/** Three transitions over the full report: figures end-aligned, the unanswered
 * one says so under its name, and the first row's evidence is open. */
export const Populated: Story = {
  render: () => (
    <Served routes={populatedRoutes}>
      <StageAutomationCard />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("table");
    await userEvent.click(
      canvas.getByRole("button", { name: /Qualifying → Proposal/ }),
    );
  },
};

/** The same report at phone width: the table scrolls with Transition pinned. */
export const PopulatedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: Populated.render,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("table");
  },
};
