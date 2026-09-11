// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import type { RecordAssignment } from "./recordassignments.queries";
import { recordAssignmentsKey } from "./recordassignments.queries";
import { RecordTeam } from "./recordteam";
import { installFetchStub, jsonResponse } from "./story-utils";

// Who is responsible for a record.
//
// The states worth seeing are the degraded ones. A retired role and a
// deactivated colleague both keep their label here, because the row is history by
// then and a name that vanished would read as a bug rather than as a
// responsibility waiting for a successor. The empty state has to say what puts
// something there, or a reader takes it for a broken panel.

const meta: Meta<typeof RecordTeam> = {
  title: "Records/Shared/Responsible",
  component: RecordTeam,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof RecordTeam>;

const RECORD_ID = "11111111-1111-1111-1111-111111111111";

const row = (over: Partial<RecordAssignment>): RecordAssignment =>
  ({
    id: "a1",
    record_type: "deal",
    record_id: RECORD_ID,
    subject_kind: "user",
    subject_id: "u1",
    subject_name: "Mara Feld",
    subject_inactive: false,
    role_id: "r1",
    role_key: "account_manager",
    role_label: "Account manager",
    role_active: true,
    version: 1,
    created_at: "2026-09-01T10:00:00Z",
    updated_at: "2026-09-01T10:00:00Z",
    ...over,
  }) as RecordAssignment;

// Both the seeded cache AND the route are answered. Data written with
// setQueryData is stale the moment it lands, so the mount fires a background
// refetch that would otherwise reach the real network and 404 — which the
// render gate's browser reports as a console error the story fails on.
function Served({
  rows,
  children,
}: Readonly<{ rows: RecordAssignment[]; children: ReactNode }>) {
  const body = { data: rows };
  installFetchStub({
    [`GET /records/deal/${RECORD_ID}/assignments`]: () => jsonResponse(body),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(recordAssignmentsKey("deal", RECORD_ID), rows);
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

/** An ordinary record: two colleagues and a team, each under its own role. */
export const Staffed: Story = {
  render: () => (
    <Served
      rows={[
        row({}),
        row({
          id: "a2",
          subject_id: "u2",
          subject_name: "Jonas Weiler",
          role_id: "r2",
          role_key: "sales_engineer",
          role_label: "Sales engineer",
        }),
        row({
          id: "a3",
          subject_kind: "team",
          subject_id: "t1",
          subject_name: "Platform support",
          role_id: "r3",
          role_key: "support_team",
          role_label: "Support team",
        }),
      ]}
    >
      <RecordTeam recordType="deal" recordId={RECORD_ID} />
    </Served>
  ),
};

/**
 * The two rows that must keep rendering: a role somebody retired, and a colleague
 * who has left. Both stay readable, and both say why they need attention.
 */
export const RetiredRoleAndInactiveColleague: Story = {
  render: () => (
    <Served
      rows={[
        row({ role_active: false }),
        row({
          id: "a2",
          subject_id: "u2",
          subject_name: "Ines Kraft",
          subject_inactive: true,
          role_id: "r2",
          role_key: "delivery_lead",
          role_label: "Delivery lead",
        }),
      ]}
    >
      <RecordTeam recordType="deal" recordId={RECORD_ID} />
    </Served>
  ),
};

/** Nobody assigned yet. The detail line says what puts somebody here. */
export const Empty: Story = {
  render: () => (
    <Served rows={[]}>
      <RecordTeam recordType="deal" recordId={RECORD_ID} />
    </Served>
  ),
};
