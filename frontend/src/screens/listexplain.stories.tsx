// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { LiveExplanation } from "./listexplain";
import {
  LIVE_ID,
  listsMe,
  liveVocabulary,
  liveWhy,
  MEMBER_ID,
  notOnLiveWhy,
} from "./lists.fixtures";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// A Live List's filter judged for one record, clause by clause, with the
// record's value beside each clause or "hidden" where the reader may not see it.
const meta: Meta = {
  title: "Records/List explanation",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function stub() {
  installFetchStub({
    "GET /me": listsMe(true),
    "GET /filters/vocabulary": () => jsonResponse(liveVocabulary),
  });
}

export const OnTheList: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <LiveExplanation entityType="company" why={liveWhy} />
      </StoryProviders>
    );
  },
};

export const NotOnTheList: Story = {
  render: () => {
    stub();
    return (
      <StoryProviders>
        <LiveExplanation entityType="company" why={notOnLiveWhy} />
      </StoryProviders>
    );
  },
};

const KEY_ACCOUNT = "01a0f000-0000-7000-8000-000000000050";
const TRADE_FAIR = "01a0f000-0000-7000-8000-000000000051";

// A clause naming a tag retired since the list was written: the clause calls it
// archived and says why it holds for no record, beside a live tag's clause.
export const RetiredTag: Story = {
  render: () => {
    installFetchStub({
      "GET /me": () =>
        jsonResponse({
          ...meFixture({
            allow: { list: ["read"], company: ["read"], tag: ["read"] },
            settingsAvailability: { lists: true },
          }),
          teams: [],
        }),
      "GET /tags": () =>
        jsonResponse({
          data: [
            { id: KEY_ACCOUNT, name: "Key account" },
            {
              id: TRADE_FAIR,
              name: "Trade fair 2025",
              archived_at: "2026-01-12T09:00:00Z",
            },
          ],
          page: { has_more: false },
        }),
      "GET /filters/vocabulary": () =>
        jsonResponse({
          resource: "company",
          fields: [
            {
              name: "tag",
              type: "id",
              operators: ["eq", "neq", "in"],
              custom: false,
              references: "tag",
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <LiveExplanation
          entityType="company"
          why={{
            list_id: LIVE_ID,
            entity_id: MEMBER_ID,
            list_type: "dynamic",
            member: true,
            eligible: true,
            clauses: {
              join: "or",
              result: true,
              children: [
                {
                  field: "tag",
                  op: "eq",
                  operand: TRADE_FAIR,
                  result: false,
                  hidden: true,
                },
                {
                  field: "tag",
                  op: "neq",
                  operand: KEY_ACCOUNT,
                  result: true,
                  hidden: true,
                },
              ],
            },
          }}
        />
      </StoryProviders>
    );
  },
};
