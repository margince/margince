// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LeadDealSection } from "./leadraillinks";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The deal slice of a lead's rail: the deal this lead became, when it became
// one, and otherwise the one verb that earns it. The three states are what a
// reader has to be able to tell apart — a deal named, a deal still to earn, and
// a closed lead where the verb no longer applies rather than standing refused.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Rail links",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const DEAL_ID = "01a03000-0000-7000-8000-000000000001";

const lead = (over: Partial<Lead> = {}): Lead =>
  ({
    id: "l-1",
    full_name: "Jonas Petersen",
    company_name: "Nordwind Logistik",
    status: "contacted",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Lead;

const section = (over: Partial<Lead> = {}, reasonId?: string) => {
  installFetchStub({
    "GET /me": meRoute({ lead: ["read", "update"], deal: ["read"] }),
    [`GET /deals/${DEAL_ID}`]: () =>
      jsonResponse({
        id: DEAL_ID,
        name: "Fleet telematics rollout",
        stage_id: "st-1",
        status: "open",
        version: 1,
      }),
  });
  return (
    <StoryProviders>
      <LeadDealSection
        lead={lead(over)}
        onQualify={() => {}}
        reasonId={reasonId}
      />
      {/* The page's one sentence about why this lead takes no writes, so the
          refused story's `reasonId` names a real element. */}
      {reasonId && <p id={reasonId}>This lead is not yours to change.</p>}
    </StoryProviders>
  );
};

/** No deal yet: the sentence says so, and Qualify is the door that earns one. */
export const StillToEarn: Story = { render: () => section() };

/** The deal the lead became. The whole card is the link, and the verb is gone. */
export const Qualified: Story = {
  render: () => section({ qualified_deal_id: DEAL_ID }),
};

/**
 * Closed. The verb is ABSENT rather than refused: a lead that can no longer be
 * qualified has nothing for that control to mean, and a disabled Qualify would
 * invite a press that was never available.
 */
export const Closed: Story = {
  render: () => section({ archived_at: "2026-09-01T00:00:00Z" }),
};

/**
 * Open, but not this reader's to change. Here the verb STAYS and says why —
 * the precondition is about the reader rather than about the record.
 */
export const Refused: Story = {
  render: () => section({}, "lead-not-yours"),
};
