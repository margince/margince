// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import { commercialApproval } from "../../testing/commercial-approval";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ApprovalRow } from "./approvalrow";

// One staged proposal as a decidable row — the canonical affordance for
// anything an agent proposes and a human decides. Several surfaces draw this
// row (the workspace queue, Brief, the company record), so it is the one place
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
  };
}

// `decide` is what the approve/reject buttons hit. A story that wants the row's
// refusal states answers that route with the problem document the server sends.
function Row({
  subject,
  locale = "en",
  decided,
  decide,
}: Readonly<{
  subject: Approval;
  locale?: Locale;
  decided?: boolean;
  decide?: () => Response;
}>) {
  installFetchStub({
    // The row gates on nothing but the viewer's identity — deciding a staged
    // proposal is authorised server-side on the effect, not on an object grant
    // this surface could read — so the grants here are the ones the DECISIONS
    // land on rather than a grant for the row itself.
    "GET /me": meRoute({ deal: ["read", "update"] }),
    "GET /users/names": () =>
      jsonResponse({
        data: [
          { id: "original", display_name: "Theo" },
          { id: "assignee", display_name: "Mira" },
        ],
      }),
    "GET /activities/22222222-2222-4222-8222-222222222222": () =>
      jsonResponse({
        id: "22222222-2222-4222-8222-222222222222",
        kind: "meeting",
        subject: "Quarterly account review",
        body: "Alex: I will send the revised scope.",
        occurred_at: "2026-09-01T09:00:00Z",
        is_done: true,
        source: "manual",
        captured_by: "human:reviewer",
        created_at: "2026-09-01T09:00:00Z",
        updated_at: "2026-09-01T09:00:00Z",
        links: [],
        version: 1,
      }),
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
    <StoryProviders locale={locale}>
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

export const ReassignedCommitment: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        requested_decider_id: "assignee",
        summary: "Send revised pricing",
        proposed_change: {
          summary: "Send revised pricing",
          party: "Theo",
          promiser_seat_id: "original",
          seat_id: "assignee",
          due_date: "2026-10-23",
        },
      })}
    />
  ),
};

export const InactivePromiser: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send revised pricing",
        proposed_change: {
          summary: "Send revised pricing",
          party: "Theo",
          promiser_seat_id: "original",
          due_date: "2026-10-23",
        },
      })}
    />
  ),
};

export const InactivePromiserDark: Story = {
  ...InactivePromiser,
  globals: { theme: "dark" },
};

export const UnresolvedPromiser: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Confirm the delivery date",
        proposed_change: {
          summary: "Confirm the delivery date",
          party: "Mira",
          promiser_unresolved: true,
        },
      })}
    />
  ),
};

function CommitmentReading({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <Row
      locale={locale}
      subject={approval({
        kind: "commitment_task",
        summary: "Send the revised scope for the fleet renewal",
        target_entity_type: "activity",
        target_entity_id: "22222222-2222-4222-8222-222222222222",
        proposed_change: {
          summary: "Send the revised scope for the fleet renewal",
          party:
            "Alexandra Beck · Regional account development and service delivery",
          promiser_unresolved: true,
        },
      })}
    />
  );
}
export const CommitmentEnglish: Story = {
  render: () => <CommitmentReading locale="en" />,
};
export const CommitmentGerman: Story = {
  render: () => <CommitmentReading locale="de" />,
};
export const CommitmentVietnamese: Story = {
  render: () => <CommitmentReading locale="vi" />,
};
export const CommitmentGermanDark: Story = {
  ...CommitmentGerman,
  globals: { theme: "dark" },
};

export const InactiveAssignee: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send the revised scope",
        assignee_inactive: true,
        proposed_change: {
          summary: "Send the revised scope",
          party: "Alex Beck",
          seat_id: "assignee",
        },
      })}
    />
  ),
};

export const CommercialDecision: Story = {
  render: () => <Row subject={commercialApproval()} />,
};

export const CommercialDecisionGerman: Story = {
  render: () => <Row locale="de" subject={commercialApproval()} />,
};

export const CommercialDecisionVietnamese: Story = {
  render: () => <Row locale="vi" subject={commercialApproval()} />,
};

export const CommercialDecisionDark: Story = {
  ...CommercialDecision,
  globals: { theme: "dark" },
};

export const StaleCommercialDecision: Story = {
  render: () => (
    <Row
      subject={commercialApproval({
        preview: {
          version: 1,
          state: "stale",
          current_version: 4,
          reviewer_reason: "requested_decider",
          changes: [],
        },
      })}
    />
  ),
};

export const AdditionalRecipient: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send the recording",
        proposed_change: {
          summary: "Send the recording",
          existing_task_id: "33333333-3333-4333-8333-333333333333",
        },
        preview: {
          version: 1,
          state: "ready",
          reviewer_reason: "record_authority",
          effects: ["attach_task_recipients"],
          changes: [
            {
              field: "existing_task_id",
              before: {
                kind: "reference",
                text: "Human wording for the recording",
                entity_type: "activity",
                entity_id: "33333333-3333-4333-8333-333333333333",
              },
              after: {
                kind: "reference",
                text: "Human wording for the recording",
                entity_type: "activity",
                entity_id: "33333333-3333-4333-8333-333333333333",
              },
            },
            {
              field: "recipients",
              before: { kind: "empty", text: "" },
              after: { kind: "text", text: "Pat Buyer" },
            },
          ],
        },
      })}
    />
  ),
};
export const AdditionalRecipientDark: Story = {
  ...AdditionalRecipient,
  globals: { theme: "dark" },
};

function lifecycleDecision(blocked: boolean): Approval {
  return approval({
    kind: "lifecycle_change",
    target_entity_type: "company",
    target_entity_id: "company-one",
    target_label: "Cedar Labs",
    summary: "Review the end of the account relationship",
    proposed_change: {
      company_id: "company-one",
      proposed_lifecycle: "former_customer",
    },
    preview: {
      version: 1,
      state: blocked ? "stale" : "ready",
      reviewer_reason: "requested_decider",
      lifecycle_evidence: {
        scope: "company",
        occurred_at: "2026-07-31T12:00:00Z",
      },
      changes: [
        {
          field: "lifecycle",
          before: { kind: "lifecycle", text: "customer" },
          after: { kind: "lifecycle", text: "former_customer" },
        },
      ],
      conflicts: blocked
        ? [
            {
              kind: "active_contract",
              summary: "An agreement remains active.",
              blocks_approval: true,
            },
          ]
        : [],
    },
  });
}
export const AccountTermination: Story = {
  render: () => <Row subject={lifecycleDecision(false)} />,
};
export const ContinuingAgreement: Story = {
  render: () => <Row subject={lifecycleDecision(true)} />,
};
export const ContinuingAgreementDark: Story = {
  ...ContinuingAgreement,
  globals: { theme: "dark" },
};
