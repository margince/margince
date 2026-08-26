// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ApprovalRow } from "./approvalrow";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// One staged proposal as a decidable row — the canonical affordance for
// anything an agent proposes and a person decides. Several surfaces draw this
// row (the workspace queue, Home, the company record), so it is the one place
// the states below are worth reading side by side.
//
// The two error states are the ones that matter and they are NOT the same
// answer: a version skew says the world moved under the proposal and it must be
// re-staged, while an already-decided says somebody else got there first and the
// row itself is stale. Offering a retry on the second would invite a second
// decision on a settled question.

const meta: Meta = {
  title: "Patterns/Decision row",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Approval = components["schemas"]["Approval"];

function approval(over: Partial<Approval>): Approval {
  return {
    id: "ap-1",
    kind: "send_email",
    status: "pending",
    proposed_by: "agent:runner",
    summary: "Send the follow-up to Anna Weber",
    proposed_change: {
      subject: "Following up on the depot quote",
      body: "Hallo Frau Weber, anbei die überarbeitete Kalkulation.",
    },
    confidence: 0.82,
    target_version: 3,
    on_behalf_of: "u-1",
    created_at: "2026-08-20T09:00:00Z",
    ...over,
  } as unknown as Approval;
}

// `decide` is what the approve/reject buttons hit. A story that wants the row's
// refusal states answers that route with the problem document the server sends.
function Row({
  subject,
  decided,
  decide,
}: Readonly<{
  subject: Approval;
  decided?: boolean;
  decide?: () => Response;
}>) {
  installFetchStub({
    "GET /me": meRoute({ approval: ["read", "update"] }),
    "GET /agent-tools": () =>
      jsonResponse({
        data: [{ name: "send_email", tier: "confirm" }],
        page: { next_cursor: null, has_more: false },
      }),
    ...(decide
      ? {
          "POST /approvals/ap-1/approve": decide,
          "POST /approvals/ap-1/reject": decide,
        }
      : {}),
  });
  return (
    <StoryProviders>
      <ApprovalRow approval={subject} decided={decided} />
    </StoryProviders>
  );
}

export const Pending: Story = {
  render: () => <Row subject={approval({})} />,
};

// A countdown is drawn only while the row is live and the proposal expires, so
// this is the state where the row ticks.
export const Expiring: Story = {
  render: () => (
    <Row subject={approval({ expires_at: "2099-01-01T00:00:00Z" })} />
  ),
};

// A decided row is read-only: the history of what was decided, with no verbs.
export const Decided: Story = {
  render: () => <Row subject={approval({ status: "approved" })} decided />,
};

// The world moved under the proposal. The honest offer is to re-stage, not to
// try the same write again against a version that no longer exists.
export const VersionSkew: Story = {
  render: () => (
    <Row
      subject={approval({})}
      decide={() =>
        jsonResponse(
          {
            type: "about:blank",
            title: "The record changed",
            status: 409,
            code: "version_skew",
            detail: "This deal was updated after the proposal was staged.",
          },
          409,
        )
      }
    />
  ),
};

// Somebody else decided it first. The row is stale rather than wrong, so it
// drops out instead of offering a retry.
export const AlreadyDecided: Story = {
  render: () => (
    <Row
      subject={approval({})}
      decide={() =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Already decided",
            status: 409,
            code: "already_decided",
            detail: "This proposal was approved by Mira Voss.",
          },
          409,
        )
      }
    />
  ),
};
