// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import type { Cited } from "./citations";
import { EvidenceSources, fromCitations } from "./evidencesources";

// The basis under a suggestion: the message it rests on as a row of its own,
// and every other record as the compact citation chips beside it.

const meta: Meta = {
  title: "Records/Record 360/Evidence sources",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const basis: Cited[] = [
  {
    entity_type: "activity",
    entity_id: "a-1",
    email_summary: {
      activity_id: "a-1",
      occurred_at: "2026-07-11T14:17:00Z",
      version: 1,
      subject: "Re: Pilot slots before the review",
      preview: "Two slots next week would work on our side.",
      counterparty: "Dana Buyer",
      direction: "inbound",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
    },
  },
  { entity_type: "deal", entity_id: "d-1", name: "Fleet renewal 2027" },
  { entity_type: "fact", entity_id: "f-1", name: "Headcount" },
];

const noop = () => undefined;

// The company page: a message drawer, a record route and a receipt drawer, so
// every row and chip opens.
export const EveryDoor: Story = {
  render: () => (
    <StoryProviders>
      <EvidenceSources
        sources={fromCitations(basis)}
        onOpenEmail={noop}
        onOpenRecord={noop}
        onOpenReceipt={noop}
      />
    </StoryProviders>
  ),
};

// The same doors in dark, where the message row and the chips must keep contrast.
export const EveryDoorDark: Story = {
  ...EveryDoor,
  globals: { theme: "dark" },
};

// A host with no receipt drawer: the fact is prose beside the deal's button.
export const NoReceiptDrawer: Story = {
  render: () => (
    <StoryProviders>
      <EvidenceSources
        sources={fromCitations(basis)}
        onOpenEmail={noop}
        onOpenRecord={noop}
      />
    </StoryProviders>
  ),
};
