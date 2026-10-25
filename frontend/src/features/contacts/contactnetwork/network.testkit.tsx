// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import type { components } from "../../../api/schema";
import { type Locale, LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { type GrantSpec, meFixture } from "../../../testing/mefixture";
import { ContactNetworkTab } from "./index";

export type ContactGraph = components["schemas"]["ContactGraph"];
export type IntroRequest = components["schemas"]["IntroRequest"];
export type Moments = Pick<
  components["schemas"]["Contact360"],
  "relationship_changes" | "sections_omitted"
>;
export type Route = NonNullable<ContactGraph["routes"]>[number];

export const CONTACT = "018f3a1b-0000-7000-8000-000000000010";

// The strings come from the catalogue rather than being typed here, so a
// rewording is a copy change and not a test failure. What these tests are about
// is how MANY times the page says a thing and whether a card is drawn at all —
// neither of which should depend on the words chosen.
export const NO_ROUTE = en["contact.intro.stripNoPath"];
// The routes panel's own heading. By ROLE, because the strip's first slot is
// named with the same words — the count and the list are one noun on two
// surfaces — and a bare text query cannot tell a heading from a card label.
export const WAYS_IN = en["contact.intro.routesTitle"];
export const routesPanel = () =>
  screen.queryByRole("heading", { name: WAYS_IN });

export function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

/**
 * The seat the tab is read as, and a signal for when it has answered.
 *
 * `/me` answers the grant map every capability hook consults, and the default
 * fixture grants nothing — so a case about a write control has to say which
 * seat is looking. `seatAnswered` is what a case asserting a control is ABSENT
 * waits on: every capability hook reads false while the snapshot is in flight,
 * so an ungranted seat and an unanswered one draw the identical page, and a
 * query run before the answer lands would pass against a page that has not yet
 * consulted the grants at all.
 */
function stub(
  graph: ContactGraph,
  allow: GrantSpec = {},
  asks: readonly IntroRequest[] = [],
  captureGaps: readonly unknown[] = [],
) {
  let answered: () => void = () => {};
  const seatAnswered = new Promise<void>((resolve) => {
    answered = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/graph")) {
        return jsonResponse(graph);
      }
      if (url.endsWith("/network")) {
        return jsonResponse({
          contact_id: CONTACT,
          colleagues: [],
          capture_gaps: captureGaps,
          capture_gaps_total: captureGaps.length,
        });
      }
      if (url.includes("/intro-requests")) {
        return jsonResponse({ data: asks });
      }
      if (url.includes("/me")) {
        answered();
        return jsonResponse(meFixture({ roles: ["rep"], allow }));
      }
      return jsonResponse({ data: [] });
    }),
  );
  return { seatAnswered };
}

export function renderTab(
  graph: ContactGraph,
  locale: Locale = "en",
  allow: GrantSpec = {},
  // The two reads beside the graph: the asks in flight, and the 360's
  // moments. Absent by default, which is what the older contacts screen
  // hands the tab.
  beside: Readonly<{
    asks?: readonly IntroRequest[];
    view?: Moments;
    captureGaps?: readonly unknown[];
  }> = {},
) {
  const { seatAnswered } = stub(graph, allow, beside.asks, beside.captureGaps);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <ContactNetworkTab contactId={CONTACT} view={beside.view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { seatAnswered };
}

export const anchor: ContactGraph["nodes"][number] = {
  id: `contact:${CONTACT}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  contact_id: CONTACT,
};
