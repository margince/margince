// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealPulse } from "./dealpulse";
import { DealSeats } from "./dealseats";

// The deal record's opening: the sentence, and the seats beside it. The four
// readings that used to sit here moved into the cockpit band
// (dealcockpit.stories.tsx), and the facts beside the name into
// dealheaderfacts.stories.tsx.
//
// The states worth seeing are the ones easy to get wrong, and each is a story
// below: a coverage read that was WITHHELD rather than empty, and a card
// still loading. Every one of them renders identically to a healthy card if
// its distinction is dropped, which is why they are here rather than a happy
// path alone.

type DealCoverage = components["schemas"]["DealCoverage"];

const meta: Meta = {
  title: "Records/Deal 360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL_ID = "01a03000-0000-7000-8000-0000000000aa";

const coverage: DealCoverage = {
  deal_id: DEAL_ID,
  stakeholders: [
    {
      contact_id: "p-1",
      contact_name: "Thorsten Ortner",
      role: "economic_buyer",
      engaged: true,
    },
    {
      contact_id: "p-2",
      contact_name: "Martina Keller",
      role: "influencer",
      engaged: false,
    },
  ],
  our_side: [],
  risks: [],
  sections_omitted: [],
};

/** A deal that needs somebody: our move, waiting since the pilot review. */
export const NeedsYou: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse
        card={
          {
            deal_id: DEAL_ID,
            story: { sentences: [] },
            reply_to: MAIL_ID,
            next: {
              action: "draft_email",
              reason: "Answer them.",
              evidence: [
                {
                  text: "Unanswered: Slots for the pilot review",
                  activity_id: MAIL_ID,
                  occurred_at: "2026-05-20T09:00:00Z",
                },
              ],
            },
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "model",
          } as components["schemas"]["DealStatusCard"]
        }
        timeline={[]}
      />
    </StoryProviders>
  ),
};

/** Nobody here is owed an answer. */
export const TheirMove: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse
        card={
          {
            deal_id: DEAL_ID,
            story: { sentences: [] },
            reply_to: null,
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "model",
          } as components["schemas"]["DealStatusCard"]
        }
        timeline={[]}
      />
    </StoryProviders>
  ),
};

/** The seats as the rail draws them, one engaged and one not. */
export const Seats: Story = {
  render: () => (
    <StoryProviders>
      <DealSeats pending={false} withheld={false} coverage={coverage} />
    </StoryProviders>
  ),
};

/** The seats were WITHHELD, not empty — a reader may not read who is on this deal. */
export const SeatsWithheld: Story = {
  render: () => (
    <StoryProviders>
      <DealSeats pending={false} withheld={true} />
    </StoryProviders>
  ),
};

/** Nothing read yet: the sentence draws nothing rather than guessing. */
export const Loading: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse card={undefined} timeline={[]} />
      <DealSeats pending={true} withheld={false} />
    </StoryProviders>
  ),
};
