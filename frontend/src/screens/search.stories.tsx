// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { SearchScreen } from "./search";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const hits = () =>
  jsonResponse({
    data: [
      {
        type: "contact",
        id: "p1",
        title: "Dana Buyer",
        snippet: "…Dana at Acme…",
        score: 0.91,
        trust_tier: "authoritative",
      },
      {
        type: "company",
        id: "o1",
        title: "Acme GmbH",
        snippet: "…Acme…",
        score: 0.82,
        trust_tier: "authoritative",
      },
      {
        type: "deal",
        id: "d1",
        title: "Acme — Platform expansion",
        snippet: "…platform…",
        score: 0.74,
        trust_tier: "authoritative",
      },
      {
        type: "lead",
        id: "l1",
        title: "Bettina Krause",
        snippet: "…mirrored from the connected system…",
        score: 0.61,
        trust_tier: "external",
      },
      // All three tiers in one list, because the badges only do their work by
      // contrast: an unverified row beside a verified one and a mirrored one is
      // the comparison a reader actually makes.
      {
        type: "contact",
        id: "p2",
        title: "Sam Unknown",
        snippet: "…no source has vouched for this…",
        score: 0.44,
        trust_tier: "unverified",
      },
    ],
    page: { next_cursor: null, has_more: false },
  });

const meta: Meta<typeof SearchScreen> = {
  title: "Records/Search",
  component: SearchScreen,
};
export default meta;
type Story = StoryObj<typeof SearchScreen>;

export const Populated: Story = {
  render: () => {
    installFetchStub({ "GET /search": hits });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};
export const Empty: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <SearchScreen q="zzz" />
      </StoryProviders>
    );
  },
};

// Every hit type the contract can return, in one list. Project, product and
// offer-template hits are new here and are the reason this story exists: the
// group list was a hand-kept literal and had silently dropped project hits the
// server was already ranking, so the review that catches the next one is a
// picture with all of them in it.
export const EveryKind: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "contact", id: "p1", title: "Dana Buyer", score: 0.91 },
            { type: "company", id: "o1", title: "Acme GmbH", score: 0.88 },
            // A partner beside a plain company, because the mark only reads by
            // contrast: it says which of the two accounts is a partner.
            {
              type: "company",
              id: "o2",
              title: "Brandt GmbH",
              score: 0.86,
              is_partner: true,
            },
            {
              type: "deal",
              id: "d1",
              title: "Acme — Platform expansion",
              score: 0.81,
            },
            {
              type: "project",
              id: "pj1",
              title: "Acme rollout",
              snippet: "ACME-CRM · Acme GmbH",
              score: 0.77,
            },
            {
              type: "product",
              id: "pr1",
              title: "Kärcher floor scrubber",
              snippet: "KAR-9910",
              score: 0.7,
            },
            {
              type: "offer_template",
              id: "ot1",
              title: "Acme rollout quote",
              score: 0.64,
            },
            { type: "lead", id: "l1", title: "Bettina Krause", score: 0.55 },
            { type: "tag", id: "t1", title: "Key account", carried_by: 7 },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// The wait. Held open by a promise that never settles, so the skeleton is what
// the screenshot catches rather than a race with the answer.
export const Loading: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () => new Promise<Response>(() => {}) as never,
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// A search that did not finish. The reader is told what happened and offered
// the retry — never an empty list, which would say the workspace holds nothing.
export const Failed: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () =>
        new Response(
          JSON.stringify({
            type: "about:blank",
            title: "Internal Server Error",
            status: 500,
            detail: "The search index is rebuilding.",
          }),
          {
            status: 500,
            headers: { "Content-Type": "application/problem+json" },
          },
        ),
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// A narrowing that found nothing. The pills stay: losing them would leave the
// reader on an empty page with no way back but the address bar.
export const NarrowedToNothing: Story = {
  render: () => {
    globalThis.location.hash = "#/search/acme?type=product";
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// The German copy, which runs 20-35% longer — the pill row is the part of this
// screen where that shows, since it wraps rather than scrolling.
export const EveryKindGerman: Story = {
  render: () => {
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "contact", id: "p1", title: "Dana Buyer", score: 0.91 },
            { type: "company", id: "o1", title: "Acme GmbH", score: 0.88 },
            {
              type: "company",
              id: "o2",
              title: "Brandt GmbH",
              score: 0.86,
              is_partner: true,
            },
            {
              type: "product",
              id: "pr1",
              title: "Kärcher Bodenreiniger",
              snippet: "KAR-9910",
              score: 0.7,
            },
            {
              type: "offer_template",
              id: "ot1",
              title: "Acme Rollout-Angebot",
              score: 0.64,
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders locale="de">
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// The word that names an account names every thread about it, and each thread
// says it more often than the account's one name field does. Unnarrowed, the
// page shows a few of each kind, the account above the mail, and a way into the
// rest of each kind the server cut — here the mail and the deals. The account
// wears its logo, and a contact the word found only through it says so.
const ACME_LOGO =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect x='8' y='8' width='48' height='48' rx='10' fill='%230e7490'/%3E%3C/svg%3E";

const mailAbout = (id: string, subject: string, day: number) => ({
  type: "activity",
  id,
  title: subject,
  score: 9 - day,
  trust_tier: "authoritative",
  email_summary: {
    activity_id: id,
    occurred_at: `2026-09-0${day}T09:15:00Z`,
    version: 1,
    subject,
    preview: "Acme asked again about the Acme renewal terms.",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 0,
  },
});

export const CompanyAboveItsMail: Story = {
  render: () => {
    globalThis.location.hash = "#/search/acme";
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [
            mailAbout("a1", "Re: Acme renewal terms", 1),
            mailAbout("a2", "Acme — revised quote", 2),
            mailAbout("a3", "Fwd: Acme rollout plan", 3),
            mailAbout("a4", "Acme kickoff agenda", 4),
            mailAbout("a5", "Re: Acme invoice question", 5),
            {
              type: "company",
              id: "o1",
              title: "Acme GmbH",
              score: 0.4,
              logo_url: ACME_LOGO,
            },
            {
              type: "contact",
              id: "p1",
              title: "Jonas Weiß",
              score: 0.1,
              works_at: { company_id: "o1", company_name: "Acme GmbH" },
            },
            {
              type: "deal",
              id: "d1",
              title: "Acme — Platform expansion",
              score: 0.3,
            },
            { type: "deal", id: "d2", title: "Acme renewal 2027", score: 0.2 },
            { type: "deal", id: "d3", title: "Acme fleet add-on", score: 0.2 },
            { type: "deal", id: "d4", title: "Acme support plan", score: 0.1 },
            {
              type: "deal",
              id: "d5",
              title: "Acme pilot, Hamburg",
              score: 0.1,
            },
          ],
          page: { next_cursor: null, has_more: false },
          // Five apiece, the page's cap, so the cut the server reports is one
          // the data could have made.
          types_with_more: ["deal", "activity"],
        }),
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};

// One kind, narrowed: that kind's ranked list, a page at a time.
export const NarrowedWithMore: Story = {
  render: () => {
    globalThis.location.hash = "#/search/acme?type=deal";
    installFetchStub({
      "GET /search": () =>
        jsonResponse({
          data: [
            { type: "deal", id: "d1", title: "Acme — Platform expansion" },
            { type: "deal", id: "d2", title: "Acme renewal 2027" },
          ],
          page: { next_cursor: "next", has_more: true },
        }),
    });
    return (
      <StoryProviders>
        <SearchScreen q="acme" />
      </StoryProviders>
    );
  },
};
