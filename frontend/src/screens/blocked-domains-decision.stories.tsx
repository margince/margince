// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  BLANK_DECISION,
  type Decision,
  DecisionDialog,
  useSetBlockedDomain,
} from "./blocked-domains-decision";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

// Where a domain is SETTLED, as against the list beside it that only reports
// where each one stands.
//
// Two states worth keeping apart: a blank form, where Save is refused because
// a decision with no sentence behind it is a decision nobody can review later;
// and one opened over a standing decision, where the sentence is already
// there and the reader is changing their mind rather than making it up.

const STANDING: Decision = {
  domain: "newsletters.example",
  admission: "suppressed",
  reason:
    "Bulk sender — every message from it is a campaign, never one contact.",
};

// The write belongs to the CARD in the product, so the story takes it from the
// same hook rather than assembling a mutation of its own: the refusal the
// dialog draws is whatever that write reports.
function OpenDialog({ initial }: Readonly<{ initial: Decision }>) {
  const set = useSetBlockedDomain();
  return <DecisionDialog initial={initial} set={set} onClose={() => {}} />;
}

// Only a seat holding `company:update` ever reaches this dialog — both verbs
// that open it are refused without the grant — so nothing in it restates a
// denial and the story routes the seat that can be there.
function dialog(initial: Decision) {
  return () => {
    installFetchStub({ "GET /me": meRoute({ company: ["read", "update"] }) });
    return (
      <StoryProviders>
        <OpenDialog initial={initial} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DecisionDialog> = {
  title: "Settings/Data/Capture rules/Refused domain decision",
  component: DecisionDialog,
};
export default meta;

type Story = StoryObj<typeof DecisionDialog>;

// Nothing typed. The Select offers the two decisions and not the open
// question — "nobody has decided" is a state a row is read in, never one
// somebody can choose — and Save stays refused until both fields hold text.
export const NothingDecidedYet: Story = { render: dialog(BLANK_DECISION) };

// Opened over a domain somebody already settled: the form carries their
// wording, so changing the standing means editing a sentence rather than
// replacing one that has quietly vanished.
export const OverAStandingDecision: Story = { render: dialog(STANDING) };

// The same form in the dark theme, where the dialog ground, the fields and
// the page behind them are three elevations a darker palette compresses.
export const OverAStandingDecisionDark: Story = {
  globals: { theme: "dark" },
  render: dialog(STANDING),
};
