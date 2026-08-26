// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../api/schema";
import {
  ApprovalDetailModal,
  DecideOutcome,
  editableSeed,
  editableStrings,
  StagedEditor,
} from "./approvaleditor";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The two slots a decision row hands out: the dialog that reads the WHOLE
// proposal, and the inline editor that rewrites the part of it this
// installation lets a person change.
//
// The editor's shape comes from the kind, not from the payload, and that is
// what the three editor stories below are for. A `held_draft` offers the words
// and nothing else — not the addressee, not the consent purpose, not the anchor,
// because those are what the approver is agreeing TO. A `close_date_correction`
// offers one date. An undeclared kind falls back to every string field as a text
// box, which is right for prose and is why the declared kinds exist.

const meta: Meta = {
  title: "Patterns/Decision editor",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Approval = components["schemas"]["Approval"];

const detail = {
  id: "ap-1",
  kind: "held_draft",
  status: "pending",
  proposed_by: "agent:runner",
  summary: "Send the follow-up to Anna Weber",
  proposed_change: {
    subject: "Following up on the depot quote",
    body: "Hallo Frau Weber, anbei die überarbeitete Kalkulation.",
    to: "anna.weber@nordwerk.example",
    purpose: "sales_followup",
  },
  confidence: 0.82,
  target_version: 3,
  created_at: "2026-08-20T09:00:00Z",
  expires_at: "2026-08-21T09:00:00Z",
} as unknown as Approval;

export const DetailModal: Story = {
  render: () => {
    installFetchStub({ "GET /approvals/ap-1": () => jsonResponse(detail) });
    return (
      <StoryProviders>
        <ApprovalDetailModal approvalId="ap-1" open onClose={() => {}} />
      </StoryProviders>
    );
  },
};

// The editor is a controlled surface: the story owns the draft the same way the
// row does, so what is typed is what would be submitted.
function Editor({
  kind,
  change,
}: Readonly<{ kind: string; change: Record<string, unknown> }>) {
  const fields = editableStrings(kind, change);
  const [draft, setDraft] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      fields.map((entry) => [
        entry.field,
        editableSeed(entry, change[entry.field]),
      ]),
    ),
  );
  return (
    <StoryProviders>
      <StagedEditor
        fields={fields}
        draft={draft}
        onChange={(field, value) =>
          setDraft((prev) => ({ ...prev, [field]: value }))
        }
        pending={false}
        onApprove={() => {}}
        onCancel={() => {}}
      />
    </StoryProviders>
  );
}

export const HeldDraftOffersOnlyTheWords: Story = {
  render: () => (
    <Editor
      kind="held_draft"
      change={{
        subject: "Following up on the depot quote",
        body: "Hallo Frau Weber, anbei die überarbeitete Kalkulation.",
        to: "anna.weber@nordwerk.example",
        purpose: "sales_followup",
      }}
    />
  ),
};

export const CloseDateOffersOneDate: Story = {
  render: () => (
    <Editor
      kind="close_date_correction"
      change={{
        expected_close_date: "2026-11-30",
        deal_id: "01a03781-9083-7000-8000-000000000000",
        basis: "The customer moved the board date to December.",
      }}
    />
  ),
};

// A date the payload spells correctly and the calendar does not have. The seed
// is the DISPLAY rule too, so the control starts empty rather than showing a day
// that would silently ride out unchanged on approve.
export const ImpossibleDateSeedsEmpty: Story = {
  render: () => (
    <Editor
      kind="close_date_correction"
      change={{ expected_close_date: "2026-02-30" }}
    />
  ),
};

// An undeclared kind: every string field as a text box.
export const UndeclaredKindFallsBackToText: Story = {
  render: () => (
    <Editor
      kind="some_new_kind_nobody_declared"
      change={{ note: "Ring them before Friday", owner: "Mira Voss" }}
    />
  ),
};

// The two refusals a decision can come back with, side by side, because they are
// different answers: a skew offers a re-read, and a generic failure says what
// went wrong without offering one.
export const OutcomeVersionSkew: Story = {
  render: () => (
    <StoryProviders>
      <DecideOutcome
        decide={{ isError: true, error: new Error("stale") }}
        skew
        alreadyDecided={false}
        onReRead={() => {}}
      />
    </StoryProviders>
  ),
};

export const OutcomeGenericFailure: Story = {
  render: () => (
    <StoryProviders>
      <DecideOutcome
        decide={{ isError: true, error: new Error("network unreachable") }}
        skew={false}
        alreadyDecided={false}
        onReRead={() => {}}
      />
    </StoryProviders>
  ),
};
