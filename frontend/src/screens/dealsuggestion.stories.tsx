// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { Panel, PanelBody } from "../design-system/panel";
import {
  AcceptSuggestionDialog,
  CompanySuggestions,
  DealSuggestionCard,
} from "./dealsuggestion";
import type { DealSuggestion } from "./dealsuggestions.queries";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A Deal Scout suggestion: a deal the evidence says an account should have.
// Drawn STAGED — the indigo edge and the agent's mark — because nobody has
// agreed to it yet. What every frame is about: the proposed deal's name, the
// amount only where a finished document reading stated one with its currency,
// the evidence in the reader's words, and the two answers.

const suggestion: DealSuggestion = {
  id: "01a0e9f1-0000-7000-8000-000000000001",
  kind: "open_deal",
  state: "open",
  company_id: "01a0e9f1-0000-7000-8000-0000000000c1",
  company_name: "Acme GmbH",
  pipeline_id: "01a0e9f1-0000-7000-8000-0000000000p1",
  stage_id: "01a0e9f1-0000-7000-8000-0000000000s1",
  name_hint: "proposal_sent",
  amount_minor: 1250000,
  currency: "EUR",
  confidence: 0.9,
  created_at: "2026-09-27T09:00:00Z",
  evidence: [
    {
      kind: "attachment",
      attachment_id: "01a0e9f1-0000-7000-8000-0000000000a1",
      occurred_at: "2026-09-26T15:00:00Z",
      title: "Angebot_2026.pdf",
    },
    {
      kind: "meeting",
      activity_id: "01a0e9f1-0000-7000-8000-0000000000m1",
      occurred_at: "2026-09-24T10:00:00Z",
      title: "Scoping workshop",
    },
  ],
};

const unpriced: DealSuggestion = {
  ...suggestion,
  id: "01a0e9f1-0000-7000-8000-000000000002",
  name_hint: "opportunity_signalled",
  amount_minor: null,
  currency: null,
  confidence: 0.7,
  evidence: [
    {
      kind: "signal",
      signal_id: "01a0e9f1-0000-7000-8000-0000000000g1",
      occurred_at: "2026-09-25T10:00:00Z",
      title: "They asked for a second phase",
    },
    {
      kind: "signal",
      signal_id: "01a0e9f1-0000-7000-8000-0000000000g2",
      occurred_at: "2026-09-27T10:00:00Z",
      title: "They committed to a start in November",
    },
  ],
};

function stub(allowDecide: boolean) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: allowDecide
            ? { deal: ["read", "create"], company: ["read"] }
            : { deal: ["read"], company: ["read"] },
        }),
      ),
    "GET /deal-suggestions": () =>
      jsonResponse({ data: [suggestion], page: { has_more: false } }),
    "GET /pipelines": () =>
      jsonResponse({
        data: [
          {
            id: suggestion.pipeline_id,
            name: "Sales",
            is_default: true,
            stages: [
              {
                id: suggestion.stage_id,
                name: "Qualified",
                semantic: "open",
                position: 1,
              },
              {
                id: "01a0e9f1-0000-7000-8000-0000000000s2",
                name: "Proposal",
                semantic: "open",
                position: 2,
              },
            ],
          },
        ],
        page: { has_more: false },
      }),
  });
}

const meta: Meta<typeof DealSuggestionCard> = {
  title: "Records/Deals/Deal suggestion",
  component: DealSuggestionCard,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DealSuggestionCard>;

// The Worklist and company page reading: the whole evidence and both answers.
export const WithEvidence: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders>
        <DealSuggestionCard suggestion={suggestion} />
      </StoryProviders>
    );
  },
};

// No finished document reading stated an amount with its currency, so none is
// proposed — the card says nothing about money rather than a guess.
export const NoAmount: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders>
        <DealSuggestionCard suggestion={unpriced} />
      </StoryProviders>
    );
  },
};

// The board's ghost card: beside a column's deals, never counted among them.
export const OnTheBoard: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders>
        <div style={{ maxWidth: 280 }}>
          <DealSuggestionCard suggestion={suggestion} compact />
        </div>
      </StoryProviders>
    );
  },
};

// A reader who may not create deals sees the suggestion and no verb.
export const ReadOnly: Story = {
  render: () => {
    stub(false);
    return (
      <StoryProviders>
        <DealSuggestionCard suggestion={suggestion} />
      </StoryProviders>
    );
  },
};

// Opening the deal: every field starts at the suggestion's own value and the
// rep corrects what is wrong.
export const AcceptDialog: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders>
        <AcceptSuggestionDialog suggestion={suggestion} onClose={() => {}} />
      </StoryProviders>
    );
  },
};

// The company page's block, beside the account's signals.
export const OnTheCompanyPage: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders>
        <CompanySuggestions companyId={suggestion.company_id} />
        <Panel title="Signals">
          <PanelBody>…</PanelBody>
        </Panel>
      </StoryProviders>
    );
  },
};

// In German, whose copy runs longer.
export const German: Story = {
  render: () => {
    stub(true);
    return (
      <StoryProviders locale="de">
        <DealSuggestionCard suggestion={suggestion} />
      </StoryProviders>
    );
  },
};
