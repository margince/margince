// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { Panel, PanelRow } from "../design-system/panel";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { SuggestionDecision } from "./worklist.suggestion";

// A Deal Scout suggestion, decided on its Worklist row — see
// worklist.suggestion.tsx for why the card reads the company's suggestions
// rather than the row alone.

type WorklistItem = components["schemas"]["WorklistItem"];

const companyId = "01a0e9f1-0000-7000-8000-0000000000c1";

function suggestionRow(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "01a0e9f1-0000-7000-8000-000000000001",
    source: "deal_suggestion",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    because: [],
    actions: ["decide", "dismiss", "open"],
    subject: { type: "company", id: companyId, label: "Acme GmbH" },
    ...over,
  };
}

const meta: Meta<typeof SuggestionDecision> = {
  title: "Records/Worklist/Deal suggestion",
  component: SuggestionDecision,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel>
          <PanelRow>
            <Story />
          </PanelRow>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SuggestionDecision>;

export const Open: Story = {
  args: { item: suggestionRow() },
  render: (args) => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            allow: { deal: ["read", "create"], company: ["read"] },
          }),
        ),
      "GET /deal-suggestions": () =>
        jsonResponse({
          data: [
            {
              id: suggestionRow().id,
              kind: "open_deal",
              state: "open",
              company_id: companyId,
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
              ],
            },
          ],
          page: { has_more: false },
        }),
    });
    return <SuggestionDecision {...args} />;
  },
};

// Somebody decided it since the page was read: nothing is left to answer.
export const AlreadyDecided: Story = {
  args: { item: suggestionRow() },
  render: (args) => {
    installFetchStub({
      "GET /deal-suggestions": () =>
        jsonResponse({ data: [], page: { has_more: false } }),
    });
    return <SuggestionDecision {...args} />;
  },
};
