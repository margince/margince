/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { type Locale, LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { PersonNetworkTab } from "./index";

// What the tab says when it has LESS to say than the full case.
//
// Both cases here were found by looking at the rendered stories rather than by
// reading the code, and both look fine in the source: the page said "nobody
// reaches this contact" twice in two weights, and drew a card headed "Pick the
// one you can actually use" above an empty list. Nothing threw, every element
// existed, and a DOM query would have passed.

type PersonGraph = components["schemas"]["PersonGraph"];

const PERSON = "018f3a1b-0000-7000-8000-000000000010";

// The strings come from the catalogue rather than being typed here, so a
// rewording is a copy change and not a test failure. What these tests are about
// is how MANY times the page says a thing and whether a card is drawn at all —
// neither of which should depend on the words chosen.
const NO_ROUTE = en["person.graph.noRoute"];
const WAYS_IN = en["person.intro.routesTitle"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

function stub(graph: PersonGraph) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/graph")) {
        return jsonResponse(graph);
      }
      if (url.includes("/intro-requests")) {
        return jsonResponse([]);
      }
      return jsonResponse({ data: [] });
    }),
  );
}

function renderTab(graph: PersonGraph, locale: Locale = "en") {
  stub(graph);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <PersonNetworkTab personId={PERSON} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const anchor: PersonGraph["nodes"][number] = {
  id: `person:${PERSON}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  person_id: PERSON,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a contact nobody here reaches", () => {
  it("says so once, not twice", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [],
      groups_omitted: [],
    });

    // The strip is this page's answer-first surface, so the sentence belongs
    // there. A paragraph under it repeating the same words made one finding
    // read as two.
    await waitFor(() => {
      expect(screen.getAllByText(NO_ROUTE).length).toBe(1);
    });
  });

  it("draws no card offering a choice of nothing", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [],
      groups_omitted: [],
    });

    await screen.findByText(NO_ROUTE);
    // "Ways in — best first, pick the one you can actually use" over an empty
    // list. The card's own comment says a heading with nothing under it is
    // worse than no card, and the condition that drew it disagreed.
    expect(screen.queryByText(WAYS_IN)).toBeNull();
  });
});

// A server that predates the candidate list answers with the singular `route`
// and no `routes`. That payload DOES have a way in, and the card is the only
// thing that can draw it — so an empty `routes` alone must not suppress it.
describe("a legacy payload with one route", () => {
  it("still draws the card that can show it", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [
        anchor,
        {
          id: "user:018f3a1b-0000-7000-8000-000000000021",
          type: "colleague",
          group: "direct",
          label: "Sofia Meier",
          user_id: "018f3a1b-0000-7000-8000-000000000021",
        },
      ],
      edges: [
        {
          from: "user:018f3a1b-0000-7000-8000-000000000021",
          to: `person:${PERSON}`,
          strength_bucket: "strong",
          interactions_90d: 14,
        },
      ],
      // The singular route's own shape, which is NOT the candidate's: it
      // carries an English `why` sentence the server wrote, and no route_type.
      route: {
        via_user_id: "018f3a1b-0000-7000-8000-000000000021",
        via_display_name: "Sofia Meier",
        why: "Sofia has corresponded with them 14 times in 90 days.",
      },
      groups_omitted: [],
    });

    // findByText throws when absent, which IS the assertion.
    await screen.findByText(WAYS_IN);
    // And it does NOT claim nobody reaches them, which is the contradiction
    // the old condition was written to avoid.
    expect(screen.queryByText(NO_ROUTE)).toBeNull();
  });
});

// The lead panel and the alternatives list draw the same route, and they must
// call a blocked state the same thing.
//
// These three states were unreachable until the server learned to report them,
// and in that time two copies of the label switch drifted: one said
// "Unavailable" where the other said "Not available". Nothing failed, because
// nothing rendered. This is what fails now.
describe("a route that cannot be asked for", () => {
  const taken: NonNullable<PersonGraph["routes"]>[number] = {
    route_id: "direct:1",
    route_type: "direct",
    via_user_id: "018f3a1b-0000-7000-8000-0000000000a1",
    via_display_name: "Lena Fischer",
    strength_bucket: "strong",
    evidence: { interactions_90d: 12, two_way: true },
    // `unavailable` on purpose: it is the state whose two copies had actually
    // drifted ("Unavailable" against "Not available"), so a test built on it
    // fails when a second spelling comes back. A state whose two old spellings
    // happened to match would pass against the drift it exists to catch.
    availability: "unavailable",
  };

  it("is named the same way wherever it is drawn", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      // Two routes, so the lead panel draws the first and the alternatives
      // card draws the second: one label switch has to serve both.
      routes: [taken, { ...taken, route_id: "direct:2" }],
      groups_omitted: [],
    });

    const label = en["person.intro.unavailable"];
    await waitFor(() => {
      expect(screen.getAllByText(label).length).toBe(2);
    });
  });

  it("offers no button that would answer 409", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [taken],
      groups_omitted: [],
    });

    await screen.findByText(en["person.intro.unavailable"]);
    expect(
      screen.queryByRole("button", { name: en["person.intro.askAction"] }),
    ).toBeNull();
  });
});
