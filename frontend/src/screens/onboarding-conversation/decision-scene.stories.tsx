// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { StoryProviders } from "../story-utils";
import type { ConversationQuestion } from "./conversation-types";
import { type CandidateFacts, DecisionScene } from "./decision-scene";
import "./conversation.css";

// The live decision on its own, without the act that asks it. Two legal
// entities the read found on one site: each card carries the registered
// address, the registry or tax identifier, the string the answer writes, and
// the page the read found it on one toggle away. The identifier and the page
// path are set in the body face like the rest of the card; only the written
// string is a `<code>`, because it is the one stored verbatim.

const QUESTION: ConversationQuestion = {
  id: "clarify:legal_name",
  i18nKey: "ob.conv.clarify.question",
  params: { question: "Which company is this installation for?" },
  options: [
    {
      value: "Gradion GmbH",
      label: "Gradion GmbH",
      writes: "Gradion GmbH",
    },
    {
      value: "Gradion Holding AG",
      label: "Gradion Holding AG",
      writes: "Gradion Holding AG",
    },
  ],
  dismissLabelKey: "ob.conv.clarify.dismiss",
};

const FACTS: Readonly<Record<string, CandidateFacts>> = {
  "Gradion GmbH": {
    meta: "Friedrichstraße 68, 10117 Berlin",
    identifier: "HRB 214365 B",
    snippet: "Gradion GmbH, Friedrichstraße 68, 10117 Berlin. HRB 214365 B.",
    source: "https://gradion.test/legal/imprint",
  },
  "Gradion Holding AG": {
    meta: "Bahnhofstrasse 21, 8001 Zürich",
    identifier: "CHE-123.456.789",
  },
};

function Scene() {
  return (
    <StoryProviders>
      <DecisionScene
        question={QUESTION}
        onAnswer={() => {}}
        onDismiss={() => {}}
        factsOf={(value) => FACTS[value] ?? null}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Scene> = {
  title: "Onboarding/Conversation/Decision scene",
  component: Scene,
};
export default meta;
type Story = StoryObj<typeof Scene>;

/** Two candidates, one with a quote behind it and one with only its facts. */
export const TwoCandidates: Story = {
  render: () => <Scene />,
};

/** The quote opened: the page it was read from is shown as host and path. */
export const EvidenceOpen: Story = {
  render: () => <Scene />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const toggle = await canvas.findByRole("button", { name: "evidence" });
    await userEvent.click(toggle);
    await canvas.findByText("gradion.test/legal/imprint");
  },
};
