// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StatCard } from "./atoms";
import { FactList } from "./factlist";
import { ReadingsGrid } from "./readingsgrid";

// Four readings of one record as cards, each a door into the tab that holds
// its detail and each carrying the rows it was read from. What the stories
// check is that the row reads as four cards of ONE record: one height across,
// folding to fewer columns rather than to a ragged tail.
const meta: Meta<typeof ReadingsGrid> = {
  title: "Design System/ReadingsGrid",
  component: ReadingsGrid,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ReadingsGrid>;

const basis = (
  <FactList
    facts={[
      {
        key: "one",
        term: "Proposal",
        value: "€185,000 for ten languages",
        note: "Sent 01 Aug",
      },
    ]}
  />
);

export const FourReadings: Story = {
  render: () => (
    <ReadingsGrid label="Where this deal stands">
      <StatCard
        label="The money"
        value="€185,000"
        detail="Offer 1042 · sent"
        numeric
        basisLabel="What this rests on"
        basis={basis}
      />
      <StatCard
        label="The close"
        value="09 Sep"
        detail="in 19 days"
        tone="warn"
      />
      <StatCard
        label="The people"
        value="1 of 3 engaged"
        detail="a champion is named"
        meter={{ filled: 1, total: 3 }}
        openLabel="Open people"
        onOpen={() => {}}
      />
      <StatCard
        label="The momentum"
        value="16 days"
        detail="since the last contact"
      />
    </ReadingsGrid>
  ),
};

export const InANarrowColumn: Story = {
  // Narrower than the record column: here four cards no longer fit across,
  // and the row folds to two by two rather than to three and a card alone on
  // a row.
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <ReadingsGrid label="Where this contact stands">
        <StatCard label="Whose move" value="Yours" tone="warn" dot />
        <StatCard
          label="Open promises"
          value="1"
          numeric
          detail="19 days late"
        />
        <StatCard label="Deals they decide" value="€227k" numeric />
        <StatCard label="Next meeting" value="None" />
      </ReadingsGrid>
    </div>
  ),
};
