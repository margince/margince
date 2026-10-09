// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { GrantSpec } from "../app/mefixture";
import { Panel } from "../design-system/panel";
import {
  type DataSubjectRequest,
  DsrDetail,
  DsrTable,
} from "./privacy.requests";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "./story-utils";

// One subject request, as the queue's table row and as the drawer it opens.

// A fixed clock, after every overdue deadline below and before the far one.
const NOW = Date.parse("2026-09-01T00:00:00Z");

const ASSIGNED: DataSubjectRequest = {
  id: "d1",
  kind: "erasure",
  subject_ref: "00000000-0000-4000-8000-0000000000c1",
  subject_label: "Lena Hoffmann",
  subject_kind: "contact",
  status: "in_progress",
  assignee_id: "u-1",
  due_at: "2026-08-01T00:00:00Z",
  created_at: "2026-07-01T00:00:00Z",
};

const EXTERNAL: DataSubjectRequest = {
  id: "d2",
  kind: "access",
  subject_ref: "partner-reference-0042@acme.test",
  subject_label: null,
  subject_kind: null,
  status: "open",
  due_at: "2099-10-01T00:00:00Z",
  created_at: "2026-08-01T00:00:00Z",
};

const CLOSED: DataSubjectRequest = {
  id: "d3",
  kind: "rectify",
  subject_ref: "00000000-0000-4000-8000-0000000000c3",
  subject_label: null,
  subject_kind: null,
  status: "fulfilled",
  resolution: "Corrected the postal address on 12 August",
  due_at: "2026-08-20T00:00:00Z",
  created_at: "2026-07-20T00:00:00Z",
};

const WORKS: GrantSpec = { privacy_request: ["read", "update"] };

const SEATS: RouteMap = {
  "GET /users/names": () =>
    jsonResponse({ data: [{ id: "u-1", display_name: "Anna Weber" }] }),
  "GET /users": () =>
    jsonResponse({
      data: [
        {
          id: "u-1",
          email: "anna@margince.test",
          display_name: "Anna Weber",
          status: "active",
          is_agent: false,
        },
      ],
      page: { next_cursor: null, has_more: false },
    }),
};

function table(rows: DataSubjectRequest[]) {
  return () => {
    stubWithSession(SEATS, WORKS);
    return (
      <StoryProviders>
        <Panel title="Privacy requests">
          <DsrTable rows={rows} nowMs={NOW} onOpen={() => {}} />
        </Panel>
      </StoryProviders>
    );
  };
}

function drawer(dsr: DataSubjectRequest, allow: GrantSpec = WORKS) {
  return () => {
    stubWithSession(SEATS, allow);
    return (
      <StoryProviders>
        <DsrDetail
          dsr={dsr}
          titleId="dsr-title"
          nowMs={NOW}
          onClose={() => {}}
          onFulfilErasure={() => {}}
        />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DsrDetail> = {
  title: "Settings/Governance/Privacy and retention/Subject request",
  component: DsrDetail,
};
export default meta;

type Story = StoryObj<typeof DsrDetail>;

export const Rows: Story = { render: table([ASSIGNED, EXTERNAL, CLOSED]) };

export const RowsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: table([ASSIGNED, EXTERNAL, CLOSED]),
};

/** An overdue erasure in progress: its holder can be handed back to nobody. */
export const Working: Story = { render: drawer(ASSIGNED) };

/** A lead subject links to the lead, never to a contact of the same id. */
export const LeadSubject: Story = {
  render: drawer({
    ...ASSIGNED,
    subject_ref: "00000000-0000-4000-8000-0000000000d4",
    subject_label: "Jonas Berg",
    subject_kind: "lead",
  }),
};

/** A request the reader may read but not work: facts only, no verbs. */
export const ReadOnly: Story = {
  render: drawer(ASSIGNED, { privacy_request: ["read"] }),
};

/** Closed, with its answer kept. */
export const Closed: Story = { render: drawer(CLOSED) };

export const WorkingPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: drawer(EXTERNAL),
};

export const WorkingDark: Story = {
  globals: { theme: "dark" },
  render: drawer(ASSIGNED),
};
