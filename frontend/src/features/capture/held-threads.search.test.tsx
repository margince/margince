/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  CAPTURE_CONTACT_PARAM,
  captureReviewTarget,
} from "../../lib/capturereview";
import { routeHash } from "../../lib/router";
import { hashWithParams } from "../../lib/urlstate";
import { heldThreadList } from "../../testing/heldthreads.fixtures";
import { renderWithQueryClient } from "../../testing/queryrender";
import { linkedInReachFixture } from "../../testing/reachfixture";
import { steppedClock } from "../../testing/steppedclock";
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
  vi.useRealTimers();
  cleanup();
  window.location.hash = "";
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
  answer: (query: URLSearchParams) => Response | Promise<Response>,
  routes: Parameters<typeof installFetchStub>[0] = {},
): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  installFetchStub({
    ...routes,
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
  const user = steppedClock();
  renderWithQueryClient(
    <StoryProviders>
      <ConnectionOutcomesCard />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", {
      name: "View Mail not shared with the team",
    }),
  );
  await screen.findByRole("dialog", { name: "Mail not shared with the team" });
  await user.click(
    screen.getByRole("button", { name: "Browse recent conversations" }),
  );
  return user;
}

// The address a contact's timeline links to.
function openFromContact(contactId: string) {
  const target = captureReviewTarget("held", contactId);
  window.location.hash = hashWithParams(routeHash(target.route), target.params);
  renderWithQueryClient(
    <StoryProviders>
      <ConnectionOutcomesCard />
    </StoryProviders>,
  );
}

const contactAsked = (asked: URLSearchParams[]) =>
  asked.filter((query) => query.get(CAPTURE_CONTACT_PARAM) === "contact-one");

// The card's own unfiltered read failing is no reason to keep the drawer shut.
it("opens the list narrowed to the contact a timeline linked from", async () => {
  const asked = serveHeld(
    (query) =>
      query.get(CAPTURE_CONTACT_PARAM)
        ? jsonResponse(
            heldThreadList([thread("t-lena", "Lena's contract", "mail-one")]),
          )
        : jsonResponse({ status: 503, title: "Unavailable" }, 503),
    {
      "GET /contacts/contact-one": () =>
        jsonResponse({ id: "contact-one", full_name: "Lena Brandt" }),
    },
  );
  openFromContact("contact-one");

  const drawer = await screen.findByRole("dialog", {
    name: "Mail not shared with the team",
  });
  expect(await within(drawer).findByText("Lena's contract")).toBeVisible();
  expect(within(drawer).getByText("Threads with")).toBeVisible();
  expect(
    await within(drawer).findByRole("link", { name: "Lena Brandt" }),
  ).toBeVisible();
  expect(contactAsked(asked)).not.toHaveLength(0);
});

it("never shows one contact's threads under the next contact's name", async () => {
  let answerSam: (response: Response) => void = () => undefined;
  const samAnswered = new Promise<Response>((resolve) => {
    answerSam = resolve;
  });
  serveHeld(
    (query) =>
      query.get(CAPTURE_CONTACT_PARAM) === "contact-two"
        ? samAnswered
        : jsonResponse(
            heldThreadList([thread("t-lena", "Lena's contract", "mail-one")]),
          ),
    {
      "GET /contacts/contact-one": () =>
        jsonResponse({ id: "contact-one", full_name: "Lena Brandt" }),
      "GET /contacts/contact-two": () =>
        jsonResponse({ id: "contact-two", full_name: "Sam Okafor" }),
    },
  );
  steppedClock();
  openFromContact("contact-one");
  const drawer = await screen.findByRole("dialog", {
    name: "Mail not shared with the team",
  });
  await within(drawer).findByText("Lena's contract");

  const next = captureReviewTarget("held", "contact-two");
  window.location.hash = hashWithParams(routeHash(next.route), next.params);
  await within(drawer).findByRole("link", { name: "Sam Okafor" });
  expect(within(drawer).queryByText("Lena's contract")).toBeNull();

  answerSam(
    jsonResponse(
      heldThreadList([thread("t-sam", "Sam's renewal", "mail-two")]),
    ),
  );
  expect(await within(drawer).findByText("Sam's renewal")).toBeVisible();
});

it("says a refused contact is refused, and offers the whole list", async () => {
  const asked = serveHeld((query) =>
    query.get(CAPTURE_CONTACT_PARAM)
      ? jsonResponse(
          { status: 404, code: "not_found", title: "Not found" },
          404,
        )
      : jsonResponse(
          heldThreadList([
            thread("t-other", "Somebody else's thread", "mail-two"),
          ]),
        ),
  );
  const user = steppedClock();
  openFromContact("contact-one");

  const drawer = await screen.findByRole("dialog", {
    name: "Mail not shared with the team",
  });
  expect(await within(drawer).findByRole("alert")).toHaveTextContent(
    "The contact is archived or not visible to you, so its threads are not listed.",
  );
  expect(within(drawer).queryByText("No held thread matches.")).toBeNull();
  expect(
    within(drawer).queryByRole("button", {
      name: "Browse recent conversations",
    }),
  ).toBeNull();
  expect(contactAsked(asked)).not.toHaveLength(0);

  await user.click(
    within(drawer).getByRole("button", { name: "Show all threads" }),
  );
  expect(window.location.hash).not.toContain(CAPTURE_CONTACT_PARAM);
  await user.click(
    await within(drawer).findByRole("button", {
      name: "Browse recent conversations",
    }),
  );
  expect(
    await within(drawer).findByText("Somebody else's thread"),
  ).toBeVisible();
  expect(within(drawer).queryByRole("alert")).toBeNull();
});

it("ends the contact filter when the drawer closes", async () => {
  serveHeld(() =>
    jsonResponse(
      heldThreadList([thread("t-lena", "Lena's contract", "mail-one")]),
    ),
  );
  const user = steppedClock();
  openFromContact("contact-one");
  await screen.findByRole("dialog", { name: "Mail not shared with the team" });

  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(window.location.hash).not.toContain(CAPTURE_CONTACT_PARAM);
  expect(window.location.hash).not.toContain("review=");
});

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
