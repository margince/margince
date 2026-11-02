// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Info } from "lucide-react";
import type { CSSProperties } from "react";
import { expect, within } from "storybook/test";
import { formatMoney } from "../format/format";
import { Badge, StatCard } from "./atoms";
import { FactList } from "./factlist";
import { IconAction } from "./iconaction";
import { StatStrip } from "./statstrip";

// StatCard — one reading with the basis it was drawn from.
//
// Its frames argue by COMPARISON — every tile the same size, in the same face,
// with its figure in the same place — so they are read together, as a set.
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
  title: "Components/Text and data display/Stat card",
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
// product says "Open →", and there is no prop to rename it — so what tells one
// from another is the DESCRIPTION, which carries the reading a door opens and
// is what a screen reader reads after the name.
//
// The first tile is the case the layering exists for. Its receipt chip sits
// over the door's stretched target: pressing the chip has to open the working
// and leave the page where it is, and only a card carrying both can show that.
export const ReadingsWithADoor: Story = {
  name: "Readings — with a door, with View records, and without",
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
        tone="warning"
        detail="across every lane this morning"
        onOpen={() => {}}
      />
      <StatCard
        label="Meetings held"
        value="6"
        detail="this week"
        opens="records"
        onOpen={() => {}}
      />
      <StatCard label="Owner" value="Carol Wagner" detail="since 14 March" />
    </div>
  ),
};

// Every shape a reading takes, at the ONE size and in the ONE face they all
// draw in. A long label wraps whole; only a detail is clamped, to two lines,
// so a basis that is a paragraph cannot set the row's height.
export const ReadingsAtOneSize: Story = {
  name: "Readings — tones, clamps and the alert tile",
  render: () => (
    <div style={row}>
      <StatCard label="Owner" value="Carol Wagner" />
      <StatCard label="Consent" value="Allowed" tone="success" />
      <StatCard label="Health" value="Watch" tone="warning" />
      <StatCard label="Payment" value="At risk" tone="danger" />
      {/* Three lines of detail in a two-line box: the third is cut, and the
          card keeps the height of the four beside it. */}
      <StatCard
        label="Engagement"
        value="Cooling"
        tone="warning"
        detail="Last inbound 12 Jun · last outbound 3 Jul · nobody here has replied since the renewal was raised, and the champion left in April"
      />
      {/* A label longer than the card is wide, with the source badge and the
          receipt chip beside it: the name wraps and the chips keep their place. */}
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

// German in a compact record strip. Compounds break at a syllable, a figure in
// words wraps at its spaces, and an amount keeps to one line.
export const LongGermanReadings: Story = {
  render: () => (
    <div lang="de" style={{ maxWidth: "44rem" }}>
      <StatStrip density="compact">
        <StatCard
          label="Wahrscheinlichkeitsbereinigter Wert"
          value={formatMoney(850000, "EUR", "de")}
          detail="E-Mail · 13. Okt. 2026"
        />
        <StatCard
          label="Umsatz · 12 Monate"
          value="Kein Wert"
          detail="Wird synchronisiert"
        />
        <StatCard label="Nächster Termin" value="Kein Termin geplant" />
        <StatCard label="Anstehende Termine" value="Nicht verbunden" />
        <StatCard
          label="Ohne Kategorie"
          value={formatMoney(3700000, "EUR", "de")}
        />
      </StatStrip>
    </div>
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText("Kein Termin geplant");
    const whole = canvasElement.querySelectorAll<HTMLElement>(
      ".stat-card-label-text, .stat-card-value",
    );
    expect(whole).toHaveLength(10);
    for (const text of whole) {
      expect(text.scrollWidth).toBeLessThanOrEqual(text.clientWidth);
      expect(text.scrollHeight).toBeLessThanOrEqual(text.clientHeight);
    }
    for (const figure of canvasElement.querySelectorAll(".stat-card-value")) {
      if (/\d/.test(figure.textContent ?? "")) {
        const lines = document.createRange();
        lines.selectNodeContents(figure);
        expect(lines.getClientRects()).toHaveLength(1);
      }
    }
  },
};

// A glyph verb in the source slot beside a tile without one: the square takes
// one line of the label row, so both labels and both figures sit level.
export const IconSource: Story = {
  render: () => (
    <div style={row}>
      <StatCard
        label="No category"
        value="€12.4k"
        detail="3 deals"
        source={
          <IconAction
            inline
            label="Explain No category"
            icon={<Info aria-hidden />}
          />
        }
      />
      <StatCard label="Commit" value="€28k" detail="4 deals" />
    </div>
  ),
};

/** A figure that counts something names it on hover and keyboard focus: a tile
 *  with a door keeps its press for the door, one without opens the card. */
export const ReadingsThatNameWhatTheyCount: Story = {
  render: () => (
    <div style={row}>
      <StatCard
        label="Open deals"
        value="€244k"
        detail="4 open"
        onOpen={() => undefined}
        peek={{
          head: "Open deals on this account",
          items: [
            {
              key: "a",
              title: "Expansion Phase 2",
              sub: "Proposal",
              right: "€95k",
            },
            {
              key: "b",
              title: "Facade retrofit",
              sub: "Proposal",
              right: "€42k",
            },
          ],
        }}
      />
      <StatCard
        label="Unanswered"
        value="2 emails"
        detail="Re: ROI discussion"
        peek={{
          head: "Emails waiting for your reply",
          items: [
            { key: "a", title: "Re: ROI discussion", sub: "Received · 19 Oct" },
          ],
        }}
      />
    </div>
  ),
};
