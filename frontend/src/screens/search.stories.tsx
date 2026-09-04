// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { SearchScreen } from "./search";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const hits = () =>
  jsonResponse({
    data: [
      {
        type: "person",
        id: "p1",
        title: "Dana Buyer",
        snippet: "…Dana at Acme…",
        score: 0.91,
        trust_tier: "authoritative",
      },
      {
        type: "organization",
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
        type: "person",
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
            { type: "person", id: "p1", title: "Dana Buyer", score: 0.91 },
            { type: "organization", id: "o1", title: "Acme GmbH", score: 0.88 },
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
            { type: "person", id: "p1", title: "Dana Buyer", score: 0.91 },
            { type: "organization", id: "o1", title: "Acme GmbH", score: 0.88 },
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
