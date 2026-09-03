// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { CompanyFieldName } from "../onboarding";
import { StoryProviders } from "../story-utils";
import { type DeckCard, ReviewDeck } from "./review-deck";

// The deck asks one card at a time, so a reviewer needs one card fixed in
// view to see what it offers on its own. `icp` never had a site to read it
// off (no evidence), `offer_summary` did (evidence and a source): the pair
// is what shows the hint and the evidence are two different sentences, not
// the same fact said twice, and that the field is answerable either way.

const NO_EVIDENCE: DeckCard = {
  field: "icp",
  question: "Ideal customer",
  required: true,
  multiline: true,
  value: "",
};

const WITH_EVIDENCE: DeckCard = {
  field: "offer_summary",
  question: "What do you sell?",
  evidence: "We build inventory tools for growing retailers.",
  source: "gradion.test/product",
  required: true,
  multiline: true,
  value: "",
};

function Deck({ card }: Readonly<{ card: DeckCard }>) {
  const [value, setValue] = useState(card.value);
  const live: DeckCard = { ...card, value };
  return (
    <StoryProviders>
      <ReviewDeck
        cards={[live]}
        cardOf={(field: CompanyFieldName) =>
          field === live.field ? live : undefined
        }
        settled={6}
        onField={(_field, next) => setValue(next)}
        onDone={() => {}}
        onReadWhole={() => {}}
        pending={false}
        disabled={false}
        digest={() => null}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Deck> = {
  title: "Onboarding/Review deck",
  component: Deck,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Deck>;

// The read came back without a candidate at all: the hint is the only thing
// telling the reader what belongs in the box.
export const HintOnly: Story = {
  render: () => <Deck card={NO_EVIDENCE} />,
};

// The read found something, so the card also carries evidence above the
// control. Both lines stay: one is a claim the site made, the other is what
// the field is for.
export const HintAndEvidence: Story = {
  render: () => <Deck card={WITH_EVIDENCE} />,
};
