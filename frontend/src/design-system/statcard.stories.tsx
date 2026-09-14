// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { Badge, StatCard } from "./atoms";
import { FactList } from "./factlist";

// StatCard — one reading with the basis it was drawn from.
//
// Split out of `atoms.stories.tsx`, where its frames sat among two dozen
// unrelated atoms: this is the one control in that file whose whole argument is
// COMPARISON — every tile the same size, in the same face, with its figure in
// the same place — and that argument is only readable when the frames are read
// as a set rather than found one at a time down a page of chips and badges.
//
// Read every frame in BOTH themes with the toolbar's Theme control. Each tone
// and the alert tint are `color-mix()`es of canonical tokens, so a tile can be
// correct in light and wrong in dark.

// The row every frame below draws into: the record page's own readings shape,
// so a tile is judged at the width it actually gets.
const row: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(11rem, 1fr))",
  gap: "0.75rem",
};

const meta: Meta<typeof StatCard> = {
  title: "Design System/StatCard",
  component: StatCard,
};
export default meta;

type Story = StoryObj<typeof StatCard>;

// The reading that OPENS something, beside the one that does not. The three
// tiles are the whole of the door's contract and only read as one system side
// by side: with a door the entire tile is the press target, so the arrow at the
// foot has to lean from anywhere on the card and a keyboard Tab has to ring the
// CARD rather than the two words at the bottom of it; without one the tile is
// inert and must show neither. The word is the component's — every door in the
// product says "Open →" — and only the accessible name differs, carrying the
// reading so five doors on one page are five different doors to a screen
// reader.
//
// The first tile is the case the layering exists for. Its receipt chip sits
// over the door's stretched target: pressing the chip has to open the working
// and leave the page where it is, and only a card carrying both can show that.
export const ReadingsWithADoor: Story = {
  name: "Readings — with a door and without",
  render: () => (
    <div style={row}>
      <StatCard
        label="The contacts"
        value="1 of 3 engaged"
        detail="a champion is named"
        meter={{ filled: 1, total: 3 }}
        onOpen={() => {}}
        basis={
          <FactList
            facts={[
              {
                key: "champion",
                term: "Champion",
                value: "Carol Wagner",
                note: "Replied twice this month.",
              },
              {
                key: "silent",
                term: "Unengaged",
                value: "Two of three",
                note: "Neither has answered since April.",
              },
            ]}
          />
        }
      />
      <StatCard
        label="Urgent"
        value="7"
        tone="warn"
        detail="across every lane this morning"
        onOpen={() => {}}
      />
      <StatCard label="Owner" value="Carol Wagner" detail="since 14 March" />
    </div>
  ),
};

// Every shape a reading takes, at the ONE size and in the ONE face they all
// draw in — read down the column and no tile is bigger than its neighbour.
//
// The two clamps are what keeps that true when the copy does not cooperate: a
// label runs to one line and a detail to two, so a reading whose basis is a
// paragraph cannot make its own card the tallest in the row. Both are visible
// here, cut mid-sentence on purpose.
export const ReadingsAtOneSize: Story = {
  name: "Readings — tones, clamps and the alert tile",
  render: () => (
    <div style={row}>
      <StatCard label="Owner" value="Carol Wagner" />
      <StatCard label="Consent" value="Allowed" tone="good" />
      <StatCard label="Health" value="Watch" tone="warn" />
      <StatCard label="Payment" value="At risk" tone="danger" />
      {/* Three lines of detail in a two-line box: the third is cut, and the
          card keeps the height of the four beside it. */}
      <StatCard
        label="Engagement"
        value="Cooling"
        tone="warn"
        detail="Last inbound 12 Jun · last outbound 3 Jul · nobody here has replied since the renewal was raised, and the champion left in April"
      />
      {/* A label longer than the card is wide, with the source badge and the
          receipt chip beside it: the NAME clamps to one line and the two chips
          keep their place at the end of the row. */}
      <StatCard
        label="Net invoiced across every subsidiary, last twelve months"
        value="€1.2m"
        detail="Offer 1042 · sent"
        source={<Badge>offline_demo</Badge>}
        basis={
          <FactList
            facts={[
              {
                key: "invoiced",
                term: "Invoiced",
                value: "€1,284,500.00",
                note: "Across 14 closed deals.",
              },
            ]}
          />
        }
      />
      {/* The one whole-tile tint: the slot is bad news by being present at all,
          so there is no figure to colour that would say the same thing. */}
      <StatCard
        label="Overdue"
        value="€48k"
        tone="danger"
        alert
        detail="oldest invoice 18 days past due"
      />
      {/* A money reading takes no flag and no second face: the digits line up
          through `tabular-nums`, not through a mono family a caller asks for. */}
      <StatCard
        label="Won lifetime"
        value="€1,284,500.00"
        detail="Across 14 closed deals"
        meter={{ filled: 11, total: 14 }}
      />
    </div>
  ),
};
