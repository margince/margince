// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { PeopleChips } from "./glance";

// The glance's key people. The card is what a reader decides from — who they
// are to the account and how to write to them — so the states worth seeing
// side by side are the ones where that answer is partial: a contact with no
// title, one with no address, and a roster longer than the glance draws.
const meta: Meta<typeof PeopleChips> = {
  title: "Records/Company 360/Key people",
  component: PeopleChips,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 420 }}>
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof PeopleChips>;
type View = components["schemas"]["Organization360"];

const strength = {
  score: 71,
  bucket: "strong" as const,
  factors: { recency: 0.9, frequency: 0.6, reciprocity: 0.8, direction: 0.8 },
};

function view(people: View["people"], omitted: string[] = []): View {
  return {
    as_of: "2026-07-13T09:00:00Z",
    organization: {
      id: "o-1",
      display_name: "Brandt Automotive GmbH",
      captured_by: "human:u1",
      source: "manual",
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-06-01T08:00:00Z",
    },
    sections_omitted: omitted,
    people,
  } as View;
}

const page = { has_more: false, next_cursor: null };

/**
 * What the card is for: each person's standing on the account and the address
 * that reaches them, so which of the three to write to is decided here rather
 * than after three round trips.
 */
export const Populated: Story = {
  render: () => (
    <PeopleChips
      loading={false}
      view={view({
        data: [
          {
            person_id: "p-1",
            full_name: "Dana Buyer",
            title: "Head of Fleet",
            primary_email: "dana@brandt.example",
            deal_roles: [],
            consent: {},
            strength,
          },
          {
            person_id: "p-2",
            full_name: "Kim Ops",
            title: "Operations",
            deal_roles: [],
            consent: {},
            strength,
          },
          {
            person_id: "p-3",
            full_name: "Maximilian von Hohenlohe-Schillingsfürst",
            title: "Geschäftsführer Einkauf und Logistik",
            primary_email: "maximilian.von.hohenlohe@brandt-automotive.example",
            deal_roles: [],
            consent: {},
            strength,
          },
        ],
        page,
      })}
      onOpenTab={() => {}}
    />
  ),
};

/**
 * A title the installation did not type: it came from a provider, and it wears
 * its receipt — the dotted underline — where it stands rather than passing as
 * the company's own record. Long enough to wrap, which a title does and the
 * name and the address above and below it do not.
 */
export const ProvidedTitle: Story = {
  render: () => (
    <PeopleChips
      loading={false}
      view={view({
        data: [
          {
            person_id: "p-1",
            full_name: "Dana Buyer",
            title: null,
            provider_title: "Vice President, Fleet Operations and Aftersales",
            title_source: "provider",
            primary_email: "dana@brandt.example",
            deal_roles: [],
            consent: {},
            strength,
          },
        ],
        page,
      })}
      onOpenTab={() => {}}
    />
  ),
};

/**
 * More people than the glance draws. The remainder is a line and not a card:
 * it names no record, and a card with nothing to open in it reads as one that
 * failed to load.
 */
export const MoreThanShown: Story = {
  render: () => (
    <PeopleChips
      loading={false}
      view={view({
        data: [
          "Dana Buyer",
          "Kim Ops",
          "Rafael Ortiz",
          "Ines Weber",
          "Tomas Halle",
        ].map((full_name, i) => ({
          person_id: `p-${i}`,
          full_name,
          deal_roles: [],
          consent: {},
          strength,
        })),
        page,
      })}
      onOpenTab={() => {}}
    />
  ),
};

/**
 * The section the reader's grants do not cover. It says so rather than drawing
 * an account that nobody works at.
 */
export const Withheld: Story = {
  render: () => (
    <PeopleChips loading={false} view={view(undefined, ["people"])} />
  ),
};
