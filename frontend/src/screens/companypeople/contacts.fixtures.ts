// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { vi } from "vitest";
import type { components } from "../../api/schema";

// Wire fixtures for the company contact list, typed against the generated
// contract rather than hand-shaped: a fixture that drifts off the contract lets
// every test go on passing while the screen reads a field the server stopped
// sending. JSX-free on purpose — `scripts/fe-uat.mjs` reads every `.tsx` under
// `src/` as a component that owes a story.

type OrganizationContact = components["schemas"]["OrganizationContact"];

/**
 * contactsFixture is one account holding all three engagement states.
 *
 * All three, because the states only matter in contrast: a fixture with one of
 * them proves a label renders, not that a reader can tell the three apart —
 * which is the whole job of this screen.
 */
export function contactsFixture(): OrganizationContact[] {
  return [
    {
      person_id: "p-dietmar",
      full_name: "Dietmar Rietsch",
      title: "Managing Director",
      engagement: "answered",
      last_inbound_at: "2026-08-28T09:00:00Z",
      last_outbound_at: "2026-08-22T09:00:00Z",
      strength: {
        score: 71,
        bucket: "strong",
        factors: {
          recency: 0.9,
          frequency: 0.7,
          reciprocity: 0.8,
          direction: 1,
        },
      },
    },
    {
      person_id: "p-anne",
      full_name: "Anne Wiegert",
      title: "Head of Operations",
      engagement: "no_reply",
      last_outbound_at: "2026-07-30T09:00:00Z",
      strength: {
        score: 18,
        bucket: "weak",
        factors: { recency: 0.3, frequency: 0.2, reciprocity: 0, direction: 0 },
      },
    },
    {
      person_id: "p-philipp",
      full_name: "Philipp Königs",
      title: "CFO",
      engagement: "untried",
      strength: {
        score: 0,
        bucket: "none",
        factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
      },
    },
  ];
}

/**
 * stubContacts answers the wire and records the URLs it was asked for, so a
 * test can assert the dials the screen actually sent rather than the ones it
 * meant to.
 */
export function stubContacts(rows: OrganizationContact[]): string[] {
  const calls: string[] = [];
  // The api client hands fetch a Request, not a string — reading `.url` is what
  // the other screen fixtures do, and a `toString()` on the Request yields
  // "[object Request]", which matches no route and renders an empty list that
  // looks exactly like a screen with no data.
  vi.stubGlobal("fetch", (input: RequestInfo | URL) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    calls.push(url);
    if (url.includes("/organizations/") && url.includes("/contacts")) {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: rows,
            page: { has_more: false, next_cursor: null },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      );
    }
    return Promise.resolve(
      new Response(JSON.stringify({ data: [], page: { has_more: false } }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );
  });
  return calls;
}
