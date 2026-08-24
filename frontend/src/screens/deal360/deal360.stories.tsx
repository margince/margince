// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { StoryProviders } from "../story-utils";
import { DealPulse } from "./dealpulse";
import { DealSeats } from "./dealseats";
import { DealStrip } from "./dealstrip";

// The deal record's opening: the sentence, and the four readings under it.
//
// The states worth seeing are the ones easy to get wrong, and each is a story
// below: a close date no human confirmed, a coverage read that was WITHHELD
// rather than empty, and a card still loading. Every one of them renders
// identically to a healthy card if its distinction is dropped, which is why
// they are here rather than a happy path alone.

type Deal = components["schemas"]["Deal"];
type DealCoverage = components["schemas"]["DealCoverage"];

const meta: Meta = {
  title: "Records/Deal360",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";
const MAIL_ID = "01a03000-0000-7000-8000-0000000000aa";

const deal = (over: Partial<Deal> = {}): Deal =>
  ({
    id: DEAL_ID,
    name: "Fleet telematics rollout",
    amount_minor: 4_500_000,
    currency: "EUR",
    status: "open",
    stalled: false,
    source: "ui",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Deal;

const coverage: DealCoverage = {
  deal_id: DEAL_ID,
  stakeholders: [
    {
      person_id: "p-1",
      person_name: "Thorsten Ortner",
      role: "economic_buyer",
      engaged: true,
    },
    {
      person_id: "p-2",
      person_name: "Martina Keller",
      role: "influencer",
      engaged: false,
    },
  ],
  our_side: [],
  risks: [],
  sections_omitted: [],
};

/** A deal that needs somebody: our move, going cold, nobody confirmed the date. */
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
      <DealStrip
        deal={deal({
          expected_close_date: "2026-09-30",
          close_date_provisional: true,
          forecast_category: "best_case",
          stalled: true,
          last_activity_at: "2026-05-20T09:00:00Z",
        })}
        coverage={coverage}
        coverageWithheld={false}
      />
    </StoryProviders>
  ),
};

/** Nobody here is owed an answer, and the date is one a human agreed. */
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
      <DealStrip
        deal={deal({
          expected_close_date: "2026-09-30",
          close_date_provisional: false,
          forecast_category: "commit",
          last_activity_at: "2026-08-23T09:00:00Z",
        })}
        coverage={coverage}
        coverageWithheld={false}
      />
    </StoryProviders>
  ),
};

/**
 * The coverage read was WITHHELD, not empty. The people card must say so —
 * rendering it as "nobody is on this deal" would report a finding from a check
 * that never ran.
 */
export const CoverageWithheld: Story = {
  render: () => (
    <StoryProviders>
      <DealStrip deal={deal()} coverageWithheld={true} />
      <DealSeats pending={false} withheld={true} />
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

/** Nothing read yet: the sentence draws nothing rather than guessing. */
export const Loading: Story = {
  render: () => (
    <StoryProviders>
      <DealPulse card={undefined} timeline={[]} />
      <DealSeats pending={true} withheld={false} />
    </StoryProviders>
  ),
};
