// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { type Cited, SentenceList } from "./citations";

// The receipts under grounded prose, in the four readings a sentence can
// produce. Deal360, Company360 and Person360 all render this same component, so
// what this page shows is what all three show.
//
// The distinction worth checking here is what a chip SAYS. A chip standing for
// one record names that record — the deal's name, the mail's subject — because
// its kind is something the reader can already see. A chip standing for several
// names the count instead: one member's name would read as though it spoke for
// the rest. Whether the chip can be opened does not enter into it, which is the
// half that was missing — an emailed citation used to render the bare word
// "activity" for want of a click.

const meta: Meta = {
  title: "Records/Grounded prose/Citations",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function cite(
  entityType: Cited["entity_type"],
  id: string,
  name?: string,
): Cited {
  return { entity_type: entityType, entity_id: id, name };
}

function sentence(text: string, evidence: Cited[]) {
  return { text, evidence };
}

// A no-op: these stories are about what the chips SAY, and a page that opens a
// record is a different surface. Passing it is what makes the routable kinds
// render as buttons, which is half of what there is to look at.
const openRecord = () => undefined;

export const Named: Story = {
  render: () => (
    <StoryProviders>
      <SentenceList
        sentences={[
          sentence("They asked for pilot slots before the review.", [
            cite("activity", "a-1", "Slots for the pilot review"),
          ]),
          sentence("The renewal is the account's largest open deal.", [
            cite("deal", "d-1", "Fleet renewal 2027"),
          ]),
        ]}
        onOpenRecord={openRecord}
      />
    </StoryProviders>
  ),
};

export const Unnamed: Story = {
  render: () => (
    <StoryProviders>
      <SentenceList
        sentences={[
          // The server sends a name when it has one; nothing invents one when
          // it does not, so the kind is what is left to say.
          sentence("Someone wrote in about the retrofit.", [
            cite("activity", "a-2"),
          ]),
        ]}
        onOpenRecord={openRecord}
      />
    </StoryProviders>
  ),
};

export const Counted: Story = {
  render: () => (
    <StoryProviders>
      <SentenceList
        sentences={[
          sentence("Three threads this month all circled the same delay.", [
            cite("activity", "a-1", "Slots for the pilot review"),
            cite("activity", "a-2", "Contract questions"),
            cite("activity", "a-3", "Retrofit timing"),
          ]),
          sentence("Their headcount and revenue both came from the profile.", [
            cite("fact", "f-1", "Headcount"),
            cite("fact", "f-2", "Revenue"),
          ]),
        ]}
        onOpenRecord={openRecord}
      />
    </StoryProviders>
  ),
};

export const Unopenable: Story = {
  render: () => (
    // No onOpenRecord: a surface with nowhere to send the reader renders every
    // citation flat, and a named record is still named.
    <StoryProviders>
      <SentenceList
        sentences={[
          sentence("They asked for pilot slots before the review.", [
            cite("activity", "a-1", "Slots for the pilot review"),
            cite("deal", "d-1", "Fleet renewal 2027"),
          ]),
        ]}
      />
    </StoryProviders>
  ),
};

export const Receipted: Story = {
  render: () => (
    // A chip carrying the record's own words: resting on it opens the quote
    // in the agent's rule, the origin and the date under it, and the way to
    // the record where it has a page. What the reader checks the claim
    // against without leaving the row.
    <StoryProviders>
      <SentenceList
        sentences={[
          sentence("You reached out 12 days ago and nobody has come back.", [
            {
              ...cite("activity", "a-1", "Slots for the pilot review"),
              quote: "Hi Anna, two slots next week would work on our side.",
              at: "2026-05-01T09:00:00Z",
              origin: "Email you sent",
            },
          ]),
          sentence('"Fleet renewal 2027" has stalled.', [
            {
              ...cite("deal", "d-1", "Fleet renewal 2027"),
              at: "2026-03-14T09:00:00Z",
              origin: "Open deal, last worked",
            },
          ]),
        ]}
        onOpenRecord={openRecord}
      />
    </StoryProviders>
  ),
};
