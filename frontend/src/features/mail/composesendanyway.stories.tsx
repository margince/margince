// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import type { InlineReview } from "./composeinlinereview";
import { SendAnywayRows } from "./composesendanyway";

// What a refused Send leaves in the composer when the send is the check. Each
// story poses one state of the review the composer reads from the 409.

type CommunicationReview = components["schemas"]["CommunicationReview"];

const REVIEW: CommunicationReview = {
  id: "r-1",
  state: "needs_context",
  kind: "single",
  reason_code: "marketing_objection",
  assessment_revision: "rev-1",
  refusals: [
    { address: "anna@acme.test", reason_code: "marketing_objection" },
    {
      address: "ben@acme.test",
      reason_code: "footer_incomplete",
      footer_findings: [
        { kind: "privacy", language: "en", missing_means: ["email"] },
      ],
    },
  ],
};

function stateOf(overrides: Partial<InlineReview> = {}): InlineReview {
  return {
    shown: true,
    review: REVIEW,
    unreviewed: false,
    loading: false,
    loadFailed: false,
    retryLoad: () => {},
    canSendAnyway: true,
    confirm: () => {},
    pending: false,
    changed: false,
    failed: false,
    ...overrides,
  };
}

function Rows({ state }: Readonly<{ state: InlineReview }>) {
  const [explanation, setExplanation] = useState("");
  return (
    <StoryProviders>
      <div className="compose-blockers">
        <SendAnywayRows
          state={state}
          explanation={explanation}
          onExplanationChange={setExplanation}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Patterns/Compose mail/Send anyway",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** The server lets this sender decide: finding, warning, reason, button. */
export const Decidable: Story = { render: () => <Rows state={stateOf()} /> };

/** The same in dark, where the warning wash is re-derived from the token. */
export const DecidableDark: Story = {
  globals: { theme: "dark" },
  render: () => <Rows state={stateOf()} />,
};

/** No decision on offer, or a warning version this build does not know. */
export const EditOnly: Story = {
  render: () => <Rows state={stateOf({ canSendAnyway: false })} />,
};

/** The decision came back against a refreshed review. */
export const Refreshed: Story = {
  render: () => <Rows state={stateOf({ changed: true })} />,
};

/** An older server named the review and the read of it failed. */
export const LoadFailed: Story = {
  render: () => <Rows state={stateOf({ review: null, loadFailed: true })} />,
};

/** The read failure in dark. */
export const LoadFailedDark: Story = {
  globals: { theme: "dark" },
  render: () => <Rows state={stateOf({ review: null, loadFailed: true })} />,
};
