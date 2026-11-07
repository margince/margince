// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { CompanyContactsList } from "./contacts";
import { domainCandidates } from "./domaincontacts.fixtures";

// The company's contacts as the rep meets them: ranked, so whoever is waiting on
// a reply comes first rather than whoever the database happened to return first.
//
// The four engagement states are the reason this gallery exists. They look
// alike in a payload and must not look alike on screen — "their mail sits
// unanswered", "we replied", "we wrote and heard nothing" and "nobody has
// tried" are four different next moves, and a reader who cannot tell them
// apart at a glance is back to reading the roster line by line.

type CompanyContact = components["schemas"]["CompanyContact"];

const page = { has_more: false, next_cursor: null };

function contact(
  name: string,
  engagement: CompanyContact["engagement"],
  overrides: Partial<CompanyContact> = {},
): CompanyContact {
  return {
    contact_id: `p-${name.toLowerCase().replace(/\W/g, "-")}`,
    full_name: name,
    engagement,
    strength: {
      score: 0,
      bucket: "none",
      factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    },
    ...overrides,
  } as CompanyContact;
}

/** The account as a rep usually finds it: one way in, and a long tail. */
const MIXED: CompanyContact[] = [
  contact("Sabine Vogel", "waiting", {
    title: "Head of Partnerships",
    last_inbound_at: "2026-08-30T09:00:00Z",
    strength: {
      score: 24,
      bucket: "weak",
      factors: { recency: 0.8, frequency: 0.1, reciprocity: 0, direction: 0 },
    },
  } as Partial<CompanyContact>),
  contact("Dietmar Rietsch", "answered", {
    title: "Managing Director",
    last_inbound_at: "2026-08-22T09:00:00Z",
    last_outbound_at: "2026-08-28T09:00:00Z",
    strength: {
      score: 71,
      bucket: "strong",
      factors: { recency: 0.9, frequency: 0.7, reciprocity: 0.8, direction: 1 },
    },
  } as Partial<CompanyContact>),
  contact("Philipp Königs", "untried", { title: "CFO" }),
  contact("Anne Wiegert", "no_reply", {
    title: "Head of Operations",
    last_outbound_at: "2026-07-30T09:00:00Z",
    strength: {
      score: 18,
      bucket: "weak",
      factors: { recency: 0.3, frequency: 0.2, reciprocity: 0, direction: 0 },
    },
  } as Partial<CompanyContact>),
  contact("Jan Roth", "untried", { title: "Workshop lead" }),
];

function stub(rows: CompanyContact[]) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read"], company: ["read"] }),
    "GET /companies/o-1/contacts": () => jsonResponse({ data: rows, page }),
  });
}

const meta = {
  title: "Records/Company 360/Contacts/Contact list",
  component: CompanyContactsList,
  parameters: { layout: "padded" },
} satisfies Meta<typeof CompanyContactsList>;

export default meta;
type Story = StoryObj<typeof meta>;

/** All four states at once, which is the comparison the design has to survive. */
export const Mixed: Story = {
  args: { companyId: "o-1" },
  decorators: [
    (Story) => {
      stub(MIXED);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};

/**
 * An account nobody has approached. Every row is untried, which is a starting
 * point rather than a problem — so the list must not read as a wall of warnings.
 */
export const NobodyApproached: Story = {
  args: { companyId: "o-1" },
  decorators: [
    (Story) => {
      stub([
        contact("Philipp Königs", "untried", { title: "CFO" }),
        contact("Jan Roth", "untried", { title: "Workshop lead" }),
        contact("Ute Sommer", "untried", { title: "Procurement" }),
      ]);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};

/** A company with nobody on file yet. */
export const Empty: Story = {
  args: { companyId: "o-1" },
  decorators: [
    (Story) => {
      stub([]);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};

/**
 * A call logged by its date alone. The row names that date and no clock, so a
 * stored noon never reads as the hour they wrote.
 */
export const DayEntry: Story = {
  args: { companyId: "o-1" },
  decorators: [
    (Story) => {
      stub([
        contact("Sabine Vogel", "waiting", {
          title: "Head of Partnerships",
          last_inbound_at: "2026-08-30T12:00:00Z",
          last_inbound_precision: "day",
        }),
      ]);
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  play: async ({ canvasElement }) => {
    const touch = await within(canvasElement).findByText(
      new RegExp(`^${en["co.contacts.theyWrote"]} · `),
    );
    await expect(touch.textContent).toMatch(/30/);
    await expect(touch.textContent).not.toMatch(/\d:\d\d/);
  },
};

/** Contacts on the company’s domain that nobody has linked to it yet. */
export const DomainContactsUnlinked: Story = {
  args: { companyId: "o-1" },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute({
          company: ["read"],
          contact: ["read", "update"],
          relationship: ["read", "create"],
        }),
        "GET /companies/o-1/contacts": () =>
          jsonResponse({ data: MIXED, page }),
        "GET /companies/o-1/link-candidates": () =>
          jsonResponse({ data: domainCandidates, truncated: false }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};

export const DomainContactsUnlinkedPhone: Story = {
  ...DomainContactsUnlinked,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
