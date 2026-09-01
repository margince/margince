// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { CompanyPeopleList } from "./contacts";

// The company's people as the rep meets them: ranked, so whoever answered is
// the first row rather than whoever the database happened to return first.
//
// The three engagement states are the reason this gallery exists. They look
// alike in a payload and must not look alike on screen — "they replied",
// "we wrote and heard nothing" and "nobody has tried" are three different next
// moves, and a reader who cannot tell them apart at a glance is back to reading
// the roster line by line.

type OrganizationContact = components["schemas"]["OrganizationContact"];

const page = { has_more: false, next_cursor: null };

function contact(
  name: string,
  engagement: OrganizationContact["engagement"],
  overrides: Partial<OrganizationContact> = {},
): OrganizationContact {
  return {
    person_id: `p-${name.toLowerCase().replace(/\W/g, "-")}`,
    full_name: name,
    engagement,
    strength: {
      score: 0,
      bucket: "none",
      factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    },
    ...overrides,
  } as OrganizationContact;
}

/** The account as a rep usually finds it: one way in, and a long tail. */
const MIXED: OrganizationContact[] = [
  contact("Dietmar Rietsch", "answered", {
    title: "Managing Director",
    last_inbound_at: "2026-08-28T09:00:00Z",
    last_outbound_at: "2026-08-22T09:00:00Z",
    strength: {
      score: 71,
      bucket: "strong",
      factors: { recency: 0.9, frequency: 0.7, reciprocity: 0.8, direction: 1 },
    },
  } as Partial<OrganizationContact>),
  contact("Philipp Königs", "untried", { title: "CFO" }),
  contact("Anne Wiegert", "no_reply", {
    title: "Head of Operations",
    last_outbound_at: "2026-07-30T09:00:00Z",
    strength: {
      score: 18,
      bucket: "weak",
      factors: { recency: 0.3, frequency: 0.2, reciprocity: 0, direction: 0 },
    },
  } as Partial<OrganizationContact>),
  contact("Jan Roth", "untried", { title: "Workshop lead" }),
];

function stub(rows: OrganizationContact[]) {
  installFetchStub({
    "GET /me": meRoute({ person: ["read"], organization: ["read"] }),
    "GET /organizations/o-1/contacts": () => jsonResponse({ data: rows, page }),
  });
}

const meta = {
  title: "Records/Company 360/People/Contact list",
  component: CompanyPeopleList,
  parameters: { layout: "padded" },
} satisfies Meta<typeof CompanyPeopleList>;

export default meta;
type Story = StoryObj<typeof meta>;

/** All three states at once, which is the comparison the design has to survive. */
export const Mixed: Story = {
  args: { orgId: "o-1" },
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
  args: { orgId: "o-1" },
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
  args: { orgId: "o-1" },
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
