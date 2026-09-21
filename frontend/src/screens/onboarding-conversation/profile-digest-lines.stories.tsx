// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import type { CompanyFieldName } from "../onboarding";
import { StoryProviders } from "../story-utils";
import type { ReviewRow } from "./company-review-state";
import { DigestLine } from "./profile-digest-lines";
import "./profile-digest.css";
// `.staging-card` is the panel-ai family's staged member and lives in the
// design system's own sheet; nothing in this line's import graph pulls it, so
// without this the unanswered row renders with no edge and no tint at all.
import "../../design-system/panel.css";

// The three readings one line of the record can carry: recorded, being decided
// right now, and not written at all. The last is the only one that is a staged
// CARD rather than a line — the deck has not written it yet, and the dashes say
// so — which is why the three belong in one view.

function row(field: CompanyFieldName, label: string, value: string): ReviewRow {
  return {
    field,
    label,
    value,
    multiline: false,
    state: value === "" ? "required" : "quoted",
    evidence: null,
    confidence: null,
    emptyHintKey: "ob.conv.triage.emptyHint",
    omissionReasonKey: null,
  };
}

function Lines({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <StoryProviders>
      <div className="pdigest-section">{children}</div>
    </StoryProviders>
  );
}

const meta: Meta<typeof Lines> = {
  title: "Onboarding/Profile digest lines",
  component: Lines,
};
export default meta;

type Story = StoryObj<typeof Lines>;

// On the record, with the page it was read off cited beside it.
export const Recorded: Story = {
  render: () => (
    <Lines>
      <DigestLine
        row={row("display_name", "Company name", "Acme Freight")}
        n={1}
      />
    </Lines>
  ),
};

// The line the deck is asking about, marked in the agent's colour. A selection
// highlight, not a card: the value is real, the attention is what moved.
export const BeingDecided: Story = {
  render: () => (
    <Lines>
      <DigestLine
        row={row("offer_summary", "What they sell", "European road freight")}
        n={2}
        active
      />
    </Lines>
  ),
};

// Nothing written, and a way back to the card that would write it. The staged
// box says the gap is the agent's to fill and nobody has answered it yet.
export const NotWritten: Story = {
  render: () => (
    <Lines>
      <DigestLine row={row("icp", "Ideal customer", "")} onSettle={() => {}} />
    </Lines>
  ),
};
