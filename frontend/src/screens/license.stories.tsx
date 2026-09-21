// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LicenseReading } from "./license";
import { StoryProviders } from "./story-utils";

// Settings → License. The reading is rendered directly rather than through the
// fetching card: every state worth looking at is a state of the ENTITLEMENT, and
// stubbing a query to reach it would put the fetch on trial instead of the
// surface.
//
// The seats are ONE reading in a stacked `SettingRow` — used against granted,
// with the bar under it — because that is one fact, and the two slots plus a
// separate bar it used to be spelled it three times. So what these stories put
// on trial is a stat card INSIDE a row: the row's label and its
// what-counts-as-a-seat description above it, and the hairline the row draws.

type Entitlement = components["schemas"]["LicenseEntitlement"];

const CHECKED_AT = "2026-08-15T09:00:00Z";

function story(entitlement: Entitlement) {
  return () => (
    <StoryProviders>
      <LicenseReading entitlement={entitlement} />
    </StoryProviders>
  );
}

const meta: Meta<typeof LicenseReading> = {
  title: "Settings/People/Seats & license/Terms",
  component: LicenseReading,
};
export default meta;
type Story = StoryObj<typeof LicenseReading>;

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

// The state this screen exists for. Three things are on trial together: the
// interrupting notice, the alert tint on the reading that caused it, and a bar
// whose value is past its own maximum — it fills and stops, and the detail line
// is what says by how much.
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
// rejected, so there is a repair behind it, and this card is the one place that
// says so — the orb stopped carrying it because an unlicensed installation wore
// permanent amber and the colour stopped meaning anything. Warn rather than
// info, and neither interrupts: over-the-grant owns the only alert here.
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

// Over the grant in dark, because this is the surface with the most colour on it
// and every derived value is a color-mix that follows the dark accent lift: the
// danger callout, the alert tint on the reading and the bar's fill are three
// different tints that have to stay apart from each other AND from the card
// under them.
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

// At 390px, where the value carries both figures on ONE line and the ellipsis
// rule decides what a reader loses if it cannot. The row folds at the same
// width, so its label and description sit above the reading rather than beside
// it.
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
