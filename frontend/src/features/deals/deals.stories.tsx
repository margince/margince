// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DealScreen, FxLine } from "./deals";

// FxLine is prop-driven (no fetch/react-query inside) — the deal 360 supplies
// the amount/rate and the installation's base currency. Rendered here converted
// into a euro base, converted into a base that is NOT the euro (the case a
// hard-coded currency got wrong on every non-euro installation), and with the
// base still unnamed, where a converted figure cannot be stated at all. The
// DealScreen stories exercise the offers panel over the shared fetch stub.
const meta: Meta = {
  title: "Records/Deal 360/Page",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

export const FxConverted: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <FxLine
        amountMinor={100000}
        baseCurrency="EUR"
        fxRateToBase="0.92"
        fxRateDate="2026-07-01"
        locale="en"
      />
    </LocaleProvider>
  ),
};

export const FxNoDate: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <FxLine
        amountMinor={250000}
        baseCurrency="CHF"
        fxRateToBase="1.17"
        fxRateDate={null}
        locale="en"
      />
    </LocaleProvider>
  ),
};

export const FxBaseUnknown: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <FxLine
        amountMinor={250000}
        baseCurrency={null}
        fxRateToBase="1.17"
        fxRateDate="2026-07-01"
        locale="en"
      />
    </LocaleProvider>
  ),
};

const deal = {
  id: "d1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

const offer = {
  id: "o1",
  deal_id: "d1",
  offer_number: "OFF-0001",
  revision: 1,
  status: "draft",
  currency: "EUR",
  net_minor: 100_000,
  tax_minor: 19_000,
  gross_minor: 119_000,
  ai_generated: false,
  line_items: [],
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

// A deal whose company, partner and amount the reader may not read. The wire
// sends each as null and names it in `masked_fields`, so every one of them has
// to read as withheld rather than as a deal nobody has linked or priced.
const withheldDeal = {
  ...deal,
  amount_minor: null,
  currency: null,
  company_id: null,
  partner_company_id: null,
  masked_fields: ["amount_minor", "company_id", "partner_company_id"],
};

// One staged move waiting on this deal's own page: what the confirm-first
// queue looks like when it has something in it. Every other DealScreen story
// answers /approvals empty, so the panel is absent from all of them.
const stagedApproval = {
  id: "ap-1",
  kind: "advance_deal",
  status: "pending",
  summary: "Move Fleet retrofit to Proposal",
  proposed_by: "agent:capture",
  target_entity_type: "deal",
  target_entity_id: "d1",
  created_at: "2026-07-01T08:00:00Z",
  evidence: [],
};

// The caller the page asks about by default. It holds no grant at all, which is
// the reading the frames below document; a frame about an AVAILABLE verb has to
// name the grant that verb reads, or the page draws a refusal.
//
// A fixture like the records above it, so it is named with them rather than
// written inline as a parameter default.
const ungrantedCaller = {
  user: { id: "u-9", display_name: "Me" },
  roles: ["rep"],
  teams: [],
};

function installDealStub(
  offers: unknown[],
  record: unknown = deal,
  approvals: unknown[] = [],
  me: unknown = ungrantedCaller,
) {
  installFetchStub({
    "GET /deals/d1": () => jsonResponse(record),
    "GET /deals/d1/offers": () =>
      jsonResponse({
        data: offers,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /deals/d1/stakeholders": () => jsonResponse(emptyPage),
    "GET /pipelines": () => jsonResponse(emptyPage),
    "GET /approvals": () =>
      jsonResponse({ data: approvals, page: { next_cursor: null } }),
    "GET /activities": () => jsonResponse(emptyPage),
    "GET /records/deal/d1/context": () =>
      jsonResponse({ anchor: { type: "deal", id: "d1" }, sections: [] }),
    "GET /me": () => jsonResponse(me),
  });
}

export const WithOffers: Story = {
  render: () => {
    installDealStub([offer]);
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};

export const NoOffers: Story = {
  render: () => {
    installDealStub([]);
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};

export const PendingApprovals: Story = {
  render: () => {
    installDealStub([offer], deal, [stagedApproval]);
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};

export const WithheldReferences: Story = {
  render: () => {
    installDealStub([], withheldDeal);
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};

// WON and writable, which is the only reading that offers all four rows: Reopen
// answers a closed deal and nothing else, and the other stories' deal carries no
// `writable`, which fails closed — a menu of four refusals would document the
// state this frame is not about.
const wonDeal = { ...deal, status: "won", stage_id: "s3", writable: true };

// The seat and the grant those verbs read alongside the row's own `writable`.
// lib/capability.ts asks all three, so a row flag on its own still refuses.
const writer = meFixture({ allow: { deal: ["read", "update", "delete"] } });

/**
 * The header's overflow, open.
 *
 * The head carries identity and ONE verb, the mail; edit, share, reopen and
 * archive are worded rows in this list, because each is a verb whose
 * consequence a reader has to read before pressing and a row is where a verb
 * can say what it does. No glyphs in it: a column of icons beside four labels
 * is decoration to scan past. Archive goes last, farthest from the press that
 * opened the menu.
 */
export const HeaderMenu: Story = {
  name: "Header overflow, open",
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // FOUND, not got: the screen renders its pending skeleton first, so a
    // synchronous read runs against a `role="status"` with no verbs in it.
    await userEvent.click(
      await canvas.findByRole("button", { name: "More actions" }),
    );
    // The panel is portalled to the body — a Panel clips its own overflow —
    // so the frame is settled when an ITEM is in the document, not the canvas.
    await within(canvasElement.ownerDocument.body).findByRole("button", {
      name: "Archive deal",
    });
  },
  render: () => {
    installDealStub([offer], wonDeal, [], writer);
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};

// A deal in Proposal well past the usual time, one buying role named of three,
// twelve days since the buyer last wrote. The deal's lead is designed around
// this shape; the stories above carry no status read.
const pipeline = {
  id: "pl",
  name: "New business",
  is_default: true,
  stages: [
    {
      id: "s0",
      pipeline_id: "pl",
      name: "Discovery",
      position: 1,
      semantic: "open",
      probability: 10,
    },
    {
      id: "s0b",
      pipeline_id: "pl",
      name: "Qualification",
      position: 2,
      semantic: "open",
      probability: 30,
    },
    {
      id: "s1",
      pipeline_id: "pl",
      name: "Proposal",
      position: 3,
      semantic: "open",
      probability: 60,
    },
    {
      id: "s2",
      pipeline_id: "pl",
      name: "Negotiation",
      position: 4,
      semantic: "open",
      probability: 80,
    },
    {
      id: "won",
      pipeline_id: "pl",
      name: "Won",
      position: 5,
      semantic: "won",
      probability: 100,
    },
    {
      id: "lost",
      pipeline_id: "pl",
      name: "Lost",
      position: 6,
      semantic: "lost",
      probability: 0,
    },
  ],
};

const drifting = {
  deal_id: "d1",
  story: {
    sentences: [
      {
        text: "Dana asked for a week on the retrofit review and has not written since.",
        evidence: [],
      },
    ],
  },
  verdict: {
    standing: "drifting",
    because: {
      sentences: [
        {
          text: "Your reply to Dana has been due for twelve days, and the offer is still a draft.",
          evidence: [],
        },
      ],
    },
  },
  stage_entered_at: "2026-06-18T09:00:00Z",
  stage_usual_days: 30,
  generated_at: "2026-08-13T08:00:00Z",
  generated_by: "model",
};

export const Drifting: Story = {
  render: () => {
    installFetchStub({
      "GET /deals/d1": () =>
        jsonResponse({ ...deal, expected_close_date: "2026-09-30" }),
      "GET /deals/d1/offers": () =>
        jsonResponse({
          data: [offer],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /deals/d1/stakeholders": () => jsonResponse(emptyPage),
      "GET /deals/d1/status": () => jsonResponse(drifting),
      "GET /deals/d1/coverage": () =>
        jsonResponse({
          deal_id: "d1",
          stakeholders: [
            { contact_id: "p-1", role: "champion", engaged: true },
          ],
          our_side: [],
          risks: [
            { kind: "coverage_gap", summary: "No economic buyer named" },
            {
              kind: "single_threaded_theirs",
              summary: "Only one contact at the buyer is engaged",
            },
          ],
          sections_omitted: [],
        }),
      "GET /pipelines": () =>
        jsonResponse({ data: [pipeline], page: { next_cursor: null } }),
      "GET /approvals": () =>
        jsonResponse({ data: [], page: { next_cursor: null } }),
      "GET /activities": () => jsonResponse(emptyPage),
      "GET /records/deal/d1/context": () =>
        jsonResponse({ anchor: { type: "deal", id: "d1" }, sections: [] }),
      "GET /me": () => jsonResponse(ungrantedCaller),
    });
    return (
      <StoryProviders>
        <DealScreen id="d1" />
      </StoryProviders>
    );
  },
};
