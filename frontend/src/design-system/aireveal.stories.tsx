// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { AiPending } from "./aipending";
import { AiReveal, useArrivalCue } from "./aireveal";
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

const BRIEF = [
  "Lena has the redlines, and nothing in them moves the €48k price.",
  "The open question is timing: the depot goes offline on the 14th, and she wants to sign before then.",
  "The next step is yours: send the phase plan by Friday.",
];

// The whole arc a reader sees: the speck rows while the model works, then the
// words arriving. Ask again to replay it.
function Arc() {
  const [asking, setAsking] = useState(true);
  const cue = useArrivalCue(asking);
  useEffect(() => {
    if (!asking) {
      return;
    }
    const answered = setTimeout(() => setAsking(false), 1800);
    return () => clearTimeout(answered);
  }, [asking]);
  return (
    <Panel
      title="This week on the deal"
      tone="ai"
      titleAction={<Button onClick={() => setAsking(true)}>Ask again</Button>}
    >
      <PanelBody>
        {asking ? (
          <AiPending label="Reading the deal room." lines={3} />
        ) : (
          <AiReveal cue={cue}>
            {BRIEF.map((sentence) => (
              <p key={sentence}>{sentence}</p>
            ))}
          </AiReveal>
        )}
      </PanelBody>
    </Panel>
  );
}

export const WaitThenArrive: Story = { render: () => <Arc /> };

// An answer that was there before the surface opened has no cue, and stands still.
export const AlreadyThere: Story = {
  args: {
    cue: null,
    children: BRIEF.map((sentence) => <p key={sentence}>{sentence}</p>),
  },
};
