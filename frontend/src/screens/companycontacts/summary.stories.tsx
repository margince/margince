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
import { CoverageBand } from "./summary";

// The reading a rep takes before picking anybody: who is the way in, which
// buying role nobody holds, how much of the account is untouched.
//
// The states here are the ones a live account rarely shows all of — a demo
// workspace grants full RBAC and seeds a complete committee — so this is the
// only place the withheld and partial readings can be looked at side by side.

type Coverage = components["schemas"]["CompanyCoverage"];

function coverage(over: Partial<Coverage>): Coverage {
  return {
    as_of: "2026-08-31T09:00:00Z",
    summary: {
      contacts_total: 26,
      waiting: 0,
      answered: 2,
      no_reply: 1,
      untried: 23,
      lapsed: 0,
    },
    deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
    selected_deal_id: "d-1",
    completeness: { committee_read: true },
    ...over,
  } as Coverage;
}

function stub(body: Coverage) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read"], contact: ["read"] }),
    "GET /companies/o-1/coverage": () => jsonResponse(body),
  });
}

const meta = {
  title: "Records/Company 360/Contacts/Coverage band",
  component: CoverageBand,
  parameters: { layout: "padded" },
} satisfies Meta<typeof CoverageBand>;

export default meta;
type Story = StoryObj<typeof meta>;

function story(body: Coverage) {
  return () => {
    stub(body);
    return (
      <StoryProviders>
        <CoverageBand
          companyId="o-1"
          accountName="Brandt GmbH"
          onNarrow={() => {}}
        />
      </StoryProviders>
    );
  };
}

/** The account a rep hopes for: a way in, and one hole to close. */
export const AWayInAndAGap: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      best_way_in: {
        contact_id: "p-1",
        full_name: "Dietmar Rietsch",
        title: "Managing Director",
        engagement: "answered",
        last_inbound_at: "2026-08-28T09:00:00Z",
      },
      committee: {
        seats: [
          {
            contact_id: "p-2",
            full_name: "Philipp Königs",
            role: "economic_buyer",
            engagement: "untried",
          },
        ],
        gaps: ["champion"],
        unlisted_seats: 0,
      },
    }),
  ),
};

/**
 * The band at a phone's width, where every slot is one full-width ROW: label
 * and basis leading, figure on the trailing edge, one hairline between and no
 * boxes at all. Two-up, a ten-character value ellipsized in the middle of
 * itself — "No champion" came out "No champ…".
 */
export const Phone: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  tags: ["uat-phone"],
  render: story(
    coverage({
      summary: {
        contacts_total: 26,
        waiting: 4,
        answered: 6,
        no_reply: 5,
        untried: 11,
        lapsed: 0,
      },
      best_way_in: {
        contact_id: "p-1",
        full_name: "Dietmar Rietsch",
        title: "Managing Director",
        engagement: "answered",
        last_inbound_at: "2026-08-28T09:00:00Z",
      },
      committee: { seats: [], gaps: ["champion"], unlisted_seats: 0 },
    }),
  ),
};

/**
 * Coverage read against the ROSTER, with both gaps under it. A bare "6" says
 * nothing about an account until the reader knows whether it holds seven
 * contacts or seventy, and the two gaps are different work: nobody has
 * approached one group at all, and the other wrote to us and is still owed
 * an answer.
 */
export const CoverageAgainstTheRoster: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      summary: {
        contacts_total: 26,
        waiting: 4,
        answered: 6,
        no_reply: 5,
        untried: 11,
        lapsed: 0,
      },
      best_way_in: {
        contact_id: "p-1",
        full_name: "Dietmar Rietsch",
        title: "Managing Director",
        engagement: "answered",
        last_inbound_at: "2026-08-28T09:00:00Z",
      },
    }),
  ),
};

/**
 * Everyone was written to and nobody replied. There is no way IN to name, and
 * the card says so rather than dressing a fourth follow-up up as an opening.
 */
export const NobodyHasAnswered: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      summary: {
        contacts_total: 12,
        waiting: 0,
        answered: 0,
        no_reply: 12,
        untried: 0,
        lapsed: 0,
      },
      committee: {
        seats: [],
        gaps: ["champion", "economic_buyer"],
        unlisted_seats: 0,
      },
    }),
  ),
};

/**
 * The committee could not be read at all — the reader's role does not carry the
 * deal grant. A third state, and the one a page must not collapse into "no
 * champion": the deal may well have one this reader cannot see.
 */
export const CommitteeWithheld: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      deals: [],
      completeness: { committee_read: false },
      best_way_in: {
        contact_id: "p-1",
        full_name: "Dietmar Rietsch",
        engagement: "answered",
      },
    }),
  ),
};

/**
 * Seats exist that this reader cannot see, so no gap is named: the account HAS
 * a committee, and reporting a hole over a partial reading would invent one.
 */
export const SeatsHidden: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      committee: { seats: [], gaps: [], unlisted_seats: 3 },
    }),
  ),
};

/**
 * A seat the product read out of the contact's own messages, beside one a
 * colleague typed.
 *
 * The whole point of the treatment is the CONTRAST: dashed indigo says a
 * machine decided this and nobody has confirmed it, and the seat next to it
 * carries none of the marking because a contact already answered. Only the read
 * one offers Confirm and Change role — agreeing with a colleague on their
 * behalf is not a verb this page has.
 */
export const SeatReadFromMessages: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
      selected_deal_id: "d-1",
      committee: {
        seats: [
          {
            contact_id: "p-1",
            full_name: "Ute Sommer",
            role: "economic_buyer",
            engagement: "answered",
            relationship_id: "r-1",
            relationship_version: 1,
            ai_suggested: true,
          },
          {
            contact_id: "p-2",
            full_name: "Jan Roth",
            role: "champion",
            engagement: "no_reply",
            relationship_id: "r-2",
            relationship_version: 1,
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    }),
  ),
};

/**
 * No open deal, so the reading has nowhere to record a role.
 *
 * The button stays visible and says why. Hidden, it would teach a reader
 * nothing about what the account is missing.
 */
export const NoDealToReadRolesOnto: Story = {
  args: { companyId: "o-1", accountName: "Brandt GmbH", onNarrow: () => {} },
  render: story(
    coverage({
      deals: [],
      committee: { seats: [], gaps: [], unlisted_seats: 0 },
    }),
  ),
};
