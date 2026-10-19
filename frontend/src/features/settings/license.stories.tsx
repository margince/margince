// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../../testing/story-utils";
import { LicenseReading, SeatUsageReading } from "./license";

// The reading renders without the fetching card. Every state worth seeing is a
// state of the entitlement, and a stubbed query would test the fetch instead.

type Entitlement = components["schemas"]["LicenseEntitlement"];

const CHECKED_AT = "2026-08-15T09:00:00Z";

function story(entitlement: Entitlement) {
  return () => (
    <StoryProviders>
      <LicenseReading entitlement={entitlement} />
    </StoryProviders>
  );
}

const meta = {
  title: "Settings/People/Seats and license/Terms",
  component: LicenseReading,
  args: {
    entitlement: {
      state: "valid",
      seats_used: 9,
      seats_granted: 10,
      over_limit: false,
      checked_at: CHECKED_AT,
    },
  },
} satisfies Meta<typeof LicenseReading>;
export default meta;
type Story = StoryObj<typeof meta>;

// The seat count for a reader who may not see the entitlement: no grant, no
// bar, and nothing that reads as "unlicensed".
export const CapacityOnly: Story = {
  render: () => (
    <StoryProviders>
      <SeatUsageReading seatsUsed={7} />
    </StoryProviders>
  ),
};

// Room left, which is what most installations look like most of the time.
export const InsideTheGrant: Story = {
  render: story({
    state: "valid",
    seats_used: 9,
    seats_granted: 10,
    over_limit: false,
    checked_at: CHECKED_AT,
  }),
};

// The state this screen exists for: the interrupting notice, the danger line,
// and a bar past its own maximum that fills and stops.
export const OverTheGrant: Story = {
  render: story({
    state: "valid",
    seats_used: 11,
    seats_granted: 10,
    over_limit: true,
    checked_at: CHECKED_AT,
  }),
};

// A license that caps nothing: a count with no limit to read it against, so the
// value is the bare count, the detail carries the word, and there is no bar at
// all. The story exists because the tempting render — a bar against zero —
// invents a limit nobody set.
export const NoSeatLimit: Story = {
  render: story({
    state: "valid",
    seats_used: 40,
    over_limit: false,
    checked_at: CHECKED_AT,
  }),
};

// No license configured, which is a supported state that runs: every development
// and CI installation is in it. It must not read as an installation that is out
// of seats.
export const Unlicensed: Story = {
  render: story({
    state: "absent",
    seats_used: 12,
    over_limit: false,
    checked_at: CHECKED_AT,
  }),
};

// Asked and told no, which is NOT the state above: a token was presented and
// rejected, so there is a repair behind it, and this card is where it is made;
// the shell banner and the agent panel's pill name it and link here. Warn
// rather than info, and neither interrupts: over-the-grant owns the only alert
// here.
export const LicenseRefused: Story = {
  render: story({
    state: "rejected",
    seats_used: 12,
    over_limit: false,
    checked_at: CHECKED_AT,
  }),
};

// The refusal in dark, where the warning callout's tint is a color-mix that follows
// the dark accent lift and has to stay apart from the card under it.
export const LicenseRefusedDark: Story = {
  globals: { theme: "dark" },
  render: story({
    state: "rejected",
    seats_used: 12,
    over_limit: false,
    checked_at: CHECKED_AT,
  }),
};

// Dark mode lifts every color-mix here. The danger callout, danger line and bar
// fill must stay apart from each other and from the card.
export const OverTheGrantDark: Story = {
  globals: { theme: "dark" },
  render: story({
    state: "valid",
    seats_used: 11,
    seats_granted: 10,
    over_limit: true,
    checked_at: CHECKED_AT,
  }),
};

// At 390px the row folds: its name and description sit above the reading
// rather than beside it.
export const OverTheGrantNarrow: Story = {
  globals: { viewport: { value: "phone" } },
  // The TAG is what drives the browser to 390px; the global alone only moves
  // the manager, so this story had been captured at 1024px (fe-uat.mjs).
  tags: ["uat-phone"],
  render: story({
    state: "valid",
    seats_used: 11,
    seats_granted: 10,
    over_limit: true,
    checked_at: CHECKED_AT,
  }),
};
