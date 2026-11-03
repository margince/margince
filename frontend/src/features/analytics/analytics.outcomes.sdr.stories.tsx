// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  REPORTING_FIXTURE_ZONE,
  reportingStoryEvaluation,
} from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { day } from "../../testing/worklist.testkit";
import { MySdrWork } from "./analytics.outcomes.sdr";
import { reportingSectionsCatalog } from "./reporting.story-fixtures";

// A rep's own SDR panel: the handoffs waiting on others, one page at a time,
// beside the lead readings and the meetings ahead.

const meta: Meta = {
  title: "Records/Reports/Analytics/My SDR work",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type SdrSummary = components["schemas"]["SdrSummary"];
type Page = components["schemas"]["SdrWaitingHandoffPage"];

const SELF = "u-rep-1";
const AS_OF = "2026-09-21T12:00:00Z";

const summary: SdrSummary = {
  as_of: AS_OF,
  scope: { kind: "owner", id: SELF, label: "Maya Lindqvist" },
  workflow: "handoff",
  handoffs: {
    waiting: 3,
    unassigned: 1,
    blocked: 1,
    returned: 0,
    oldest_waiting_since: "2026-09-18T12:00:00Z",
  },
  upcoming_meetings: 2,
};

const waiting: Page = {
  as_of: AS_OF,
  rows: [
    {
      id: "h-1",
      waiting_since: "2026-09-18T12:00:00Z",
      resubmitted: false,
      restricted: false,
      subject: { type: "contact", id: "c-1", label: "Ines Duarte" },
      recipient: { id: "u-ae", label: "Sam Okafor", active: true },
    },
    {
      id: "h-2",
      waiting_since: "2026-09-20T12:00:00Z",
      resubmitted: false,
      restricted: true,
    },
  ],
  next_cursor: "cursor-2",
};

/** Two handoffs waiting, with a further page behind "Show more". */
export const HandoffsWaiting: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read"], report_definition: ["read"] }),
      "GET /analytics/metrics": () => jsonResponse(reportingSectionsCatalog),
      "GET /analytics/evaluate": () => jsonResponse(reportingStoryEvaluation),
      "GET /analytics/sdr-summary": () => jsonResponse(summary),
      "GET /analytics/sdr-summary/handoffs": () => jsonResponse(waiting),
      "GET /analytics/sdr-summary/meetings": () =>
        jsonResponse({
          as_of: AS_OF,
          window: {
            start_at: AS_OF,
            end_at: "2026-10-04T22:00:00Z",
            timezone: REPORTING_FIXTURE_ZONE,
          },
          rows: [],
        }),
      "GET /worklist": () => jsonResponse(day()),
    });
    return (
      <StoryProviders>
        <MySdrWork self={SELF} locale="en" />
      </StoryProviders>
    );
  },
};
