/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { heldThreadList } from "../../testing/heldthreads.fixtures";
import { renderWithQueryClient } from "../../testing/queryrender";
import { linkedInReachFixture } from "../../testing/reachfixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectionOutcomesCard } from "./connection-outcomes";

type HeldThread = components["schemas"]["HeldThread"];

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

const thread = (key: string, subject: string, id: string): HeldThread => ({
  thread_key: key,
  activity_id: id,
  status: "held",
  pending: false,
  attempts: 1,
  has_message: true,
  kind: "legal",
  subject,
  occurred_at: "2026-08-29T15:40:00Z",
  participants: [],
  participant_count: 0,
});

// Answers the held list by the query it is asked, and records each one.
function serveHeld(
  answer: (query: URLSearchParams) => Response,
): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /capture/senders": () => jsonResponse({ data: [] }),
    "GET /capture/contacts-awaiting-decision": () => jsonResponse({ data: [] }),
    "GET /me/linkedin-reach": () => jsonResponse(linkedInReachFixture()),
  });
  const routed = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    if (url.pathname.endsWith("/capture/held-threads")) {
      asked.push(url.searchParams);
      return Promise.resolve(answer(url.searchParams));
    }
    return routed(input, init);
  }) as typeof fetch;
  return asked;
}

async function openDrawer() {
  const user = userEvent.setup();
  renderWithQueryClient(
    <StoryProviders>
      <ConnectionOutcomesCard />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "View Held threads" }),
  );
  await screen.findByRole("dialog", { name: "Held threads" });
  return user;
}

it("asks the server for what is typed and lists what it answers", async () => {
  const all = [
    thread("t-lena", "Lena's contract", "01a05500-0000-7000-8000-0000000000c1"),
    thread("t-sam", "Sam's renewal", "01a05500-0000-7000-8000-0000000000c2"),
  ];
  const asked = serveHeld((query) =>
    jsonResponse(
      heldThreadList(
        query.get("q")
          ? all.filter((row) => row.subject?.includes("Lena"))
          : all,
      ),
    ),
  );
  const user = await openDrawer();
  expect(await screen.findByText("Sam's renewal")).toBeInTheDocument();

  await user.type(
    screen.getByRole("searchbox", { name: "Search subject or party" }),
    "lena",
  );

  await waitFor(() =>
    expect(screen.queryByText("Sam's renewal")).not.toBeInTheDocument(),
  );
  expect(screen.getByText("Lena's contract")).toBeInTheDocument();
  expect(asked.some((query) => query.get("q") === "lena")).toBe(true);
});

it("says how many threads stalled across the whole list, not this page", async () => {
  serveHeld(() =>
    jsonResponse(
      heldThreadList(
        [
          thread(
            "t-one",
            "One of many",
            "01a05500-0000-7000-8000-0000000000c3",
          ),
        ],
        {
          counts: {
            pending: 5,
            unsure: 0,
            held: 1,
            held_by_owner: 0,
            stalled: 5,
          },
        },
      ),
    ),
  );
  await openDrawer();
  expect(
    await screen.findByText(/5 threads were asked about repeatedly/),
  ).toBeInTheDocument();
});
