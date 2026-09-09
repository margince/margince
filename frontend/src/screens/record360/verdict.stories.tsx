// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../design-system/panel";
import { StoryProviders } from "../story-utils";
import { Proof, SignalStrip, VerdictHead } from "./verdict";

// The head of a record's reading: the call, the line that says why, and the
// working at the far end of it — with the findings a reader scans underneath.
//
// The working's trigger is the one thing in this head a reader can operate,
// and it is the disclosure of a MACHINE's claim, so it wears Button's aiQuiet
// variant: tinted and outlined, quiet enough not to compete with the display
// word beside it. What the stories are for is that balance, and the head's
// three columns — a call with no sentence still has to put the trigger at the
// far end rather than in the middle of the row.
//
// Shut on arrival, because that is how a reader meets it. Opening it is the
// reviewer's press.

const meta: Meta<typeof VerdictHead> = {
  title: "Records/Record reading/Verdict head",
  component: VerdictHead,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof VerdictHead>;

const promise = {
  key: "promise",
  quote:
    "I'll send you the breakdown of the proportional calculation for line item 3 this week.",
  from: "Meeting transcript · 05/08/2026 · Lena Fischer",
};

const restsOn = [
  promise,
  {
    key: "mail",
    quote: "No outgoing email with an attachment to him since 01/08/2026.",
    from: "Mailbox scan · 24/08/2026",
  },
];

function Head({
  because,
}: Readonly<{
  because?: string;
}>) {
  return (
    <StoryProviders>
      <div style={{ maxWidth: 860 }}>
        <Panel tone="ai" title="Brandt Automotive GmbH · 360">
          <VerdictHead
            label="Your move"
            tone="warn"
            because={because}
            restsOn={restsOn}
          />
          <SignalStrip
            signals={[
              {
                key: "cold",
                label: "Going cold",
                figure: "84 days",
                tone: "warn",
              },
              {
                key: "promise",
                label: "Promise overdue",
                figure: "19 days",
                tone: "danger",
              },
              { key: "reach", label: "One route in", tone: "accent" },
            ]}
          />
        </Panel>
      </div>
    </StoryProviders>
  );
}

export const CallAndWorking: Story = {
  render: () => (
    <Head because="You owe Frédéric the line-item 3 breakdown. Promised on 5 August, 19 days ago." />
  ),
};

export const CallAndWorkingDark: Story = {
  ...CallAndWorking,
  globals: { theme: "dark" },
};

// No sentence to carry the middle column: the working still belongs at the far
// end of the head, which is why it names its column rather than being placed.
export const CallWithoutASentence: Story = { render: () => <Head /> };

// The same disclosure where a row rather than a head makes the claim: no
// count, because a row's proof is one quote and a figure on it is noise.
export const WorkingBehindOneClaim: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 520 }}>
        <Panel tone="ai" title="Why Margince put this here">
          <PanelBody>
            <Proof label="Why Margince put this here" items={[promise]} />
          </PanelBody>
        </Panel>
      </div>
    </StoryProviders>
  ),
};
