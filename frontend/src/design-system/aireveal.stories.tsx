// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { AiPending } from "./aipending";
import { AiReveal } from "./aireveal";
import { Button } from "./button";
import { Panel, PanelBody } from "./panel";

const meta: Meta<typeof AiReveal> = {
  title: "Components/AI and provenance/AI reveal",
  component: AiReveal,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof AiReveal>;

// Three readings of one deal, so each "Ask again" brings words not yet seen.
const BRIEFS = [
  [
    "Lena has the redlines, and nothing in them moves the €48k price.",
    "The open question is timing: the depot goes offline on the 14th, and she wants to sign before then.",
    "The next step is yours: send the phase plan by Friday.",
  ],
  [
    "Procurement wants the depot downtime in writing before the 14th.",
    "Two of five buyers have opened the proposal; finance has not.",
  ],
  [
    "Marco asked what Bremen would cost on the same terms.",
    "A second site would double the first phase.",
  ],
];

// The whole arc a reader sees: the speck rows while the model works, then the
// words arriving the first time they are on screen.
function Arc() {
  const [asked, setAsked] = useState(0);
  const [asking, setAsking] = useState(true);
  useEffect(() => {
    if (!asking) {
      return;
    }
    const answered = setTimeout(() => setAsking(false), 1800);
    return () => clearTimeout(answered);
  }, [asking]);
  const brief = BRIEFS[asked % BRIEFS.length] ?? [];
  return (
    <Panel
      title="This week on the deal"
      tone="ai"
      titleAction={
        <Button
          onClick={() => {
            setAsked((n) => n + 1);
            setAsking(true);
          }}
        >
          Ask again
        </Button>
      }
    >
      <PanelBody>
        {asking ? (
          <AiPending label="Reading the deal room." lines={3} />
        ) : (
          <AiReveal text={brief.join("\n")}>
            {brief.map((sentence) => (
              <p key={sentence}>{sentence}</p>
            ))}
          </AiReveal>
        )}
      </PanelBody>
    </Panel>
  );
}

export const WaitThenArrive: Story = { render: () => <Arc /> };
