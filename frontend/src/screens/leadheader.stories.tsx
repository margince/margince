// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LeadPulse, LeadSubtitle } from "./leadheader";
import { StoryProviders } from "./story-utils";

// The head of a lead record: what it does and where, then the two pills that
// say what kind of record this is and where it stands. The pills are the reason
// the file has a story — a terminal lead reads in the closure's own tone, and
// which tone that is has to be read off the screen rather than off the switch.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Head",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead = (over: Partial<Lead> = {}): Lead =>
  ({
    id: "l-1",
    full_name: "Jonas Petersen",
    email: "jonas@nordwind.example",
    company_name: "Nordwind Logistik",
    title: "Head of Fleet",
    status: "contacted",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Lead;

const head = (over: Partial<Lead> = {}) => (
  <StoryProviders>
    <LeadSubtitle lead={lead(over)} />
    <LeadPulse lead={lead(over)} />
  </StoryProviders>
);

/** A lead being worked: the accent pill says somebody has written to them. */
export const Contacted: Story = { render: () => head() };

/** Answered — the one status that is good news, in the success tone. */
export const Engaged: Story = { render: () => head({ status: "engaged" }) };

/**
 * A closed lead. Promoted and disqualified are both terminal, so both read in
 * the warning tone: the pill has to agree with the band and the readings tile
 * about what a closed lead looks like, and the thing to check is that the two
 * pills still read as a pair rather than as one status and one shout.
 */
export const Promoted: Story = { render: () => head({ status: "promoted" }) };

export const Disqualified: Story = {
  render: () => head({ status: "disqualified" }),
};

/** A lead with neither a title nor a company draws no subtitle at all. */
export const NoSubtitle: Story = {
  render: () => head({ title: null, company_name: null }),
};

/** The same pills in dark, where every tone is re-derived. */
export const PromotedDark: Story = {
  globals: { theme: "dark" },
  render: () => head({ status: "promoted" }),
};
