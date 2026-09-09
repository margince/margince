// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { LeadScreen } from "./leads";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

type Lead = components["schemas"]["Lead"];

// The band under the lead's header, in the two states it has anything to say
// in: a write the server refused, and the sentence that says the lead is
// closed with the way back beside it.
//
// There is deliberately no frame for a live lead nobody refused — the band
// answers `undefined` there, and a story showing the empty one would be a
// picture of the dead air it exists not to leave.
//
// Driven through LeadScreen rather than by calling `leadBand` with a writer
// made up here: the band's whole input is the page's write, and a fabricated
// one would only prove the band draws what it is handed.

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u-1",
  // Absent means NOT writable per the contract, and every state below is
  // about a reader who may write this lead.
  writable: true,
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-20T08:00:00Z",
};

const meta: Meta = {
  title: "Records/Leads/Band",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function bandStory(fixture: Lead, extra: RouteMap = {}) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ lead: ["read", "update"], activity: ["read"] }),
      "GET /leads/l-1": () => jsonResponse(fixture),
      ...extra,
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  };
}

// The closed sentence names WHICH closure, and the way back sits beside it.
// Reopen is offered on a disqualification and not on a promotion: the promoted
// closure is the demote's to reverse.
export const ClosedWithReopen: Story = {
  render: bandStory({
    ...lead,
    status: "disqualified",
    disqualify_reason: "No budget this year",
    archived_at: "2026-07-13T00:00:00Z",
  }),
};

// A refused write, stated once for the whole page rather than beside whichever
// control started it — the ladder is the page's one-click write, so it is the
// shortest way to the refusal the band has to carry.
export const WriteRefused: Story = {
  render: bandStory(lead, {
    "PATCH /leads/l-1": () =>
      jsonResponse(
        {
          title: "Conflict",
          status: 409,
          detail: "This lead changed while you were reading it.",
        },
        409,
      ),
  }),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByTestId("lead-step-engaged"),
    );
  },
};
