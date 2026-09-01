/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { en } from "../i18n/en";
import { IntroAsksCard } from "./introasks";

// The asks card is where a colleague's answer becomes words on a screen, so
// what it must never do is overstate one. A name-drop rendered as an
// introduction would tell a rep a handshake happened that did not.

const VIEWER = "u-introducer";

function ask(over: Record<string, unknown> = {}) {
  return {
    id: "ir-1",
    person_id: "p-1",
    requester_user_id: "u-requester",
    introducer_user_id: VIEWER,
    route_type: "direct",
    internal_reason: "she ran the migration we are pitching",
    status: "requested",
    name_drop_allowed: false,
    fallback_policy: "none",
    note_generated_by: "human",
    note_ai_generated: false,
    requested_at: "2026-09-01T09:00:00Z",
    due_at: "2026-09-08T09:00:00Z",
    version: 1,
    ...over,
  };
}

function mockFetch(asks: unknown[], viewer: string | null = VIEWER) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      // openapi-fetch hands the shim a Request, so the path is on .url —
      // stringifying the object itself yields "[object Request]" and matches
      // nothing, which renders as a card that simply never loads.
      const path = typeof input === "string" ? input : (input as Request).url;
      if (path.includes("/intro-requests")) {
        return new Response(JSON.stringify({ data: asks }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      if (path.endsWith("/me")) {
        return new Response(
          JSON.stringify({ user: { id: viewer }, capabilities: {} }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response("{}", {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <IntroAsksCard personId="p-1" personName="Dana" />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("renders a name-drop as a name-drop and never as an introduction", async () => {
  mockFetch([ask({ status: "name_dropped" })]);
  renderCard();

  await waitFor(() => {
    expect(screen.getByText(en["person.intro.stateNameDropped"])).toBeTruthy();
  });
  // The one sentence this card must never show for a name-drop.
  expect(screen.queryByText(en["person.intro.stateIntroduced"])).toBeNull();
});

it("offers the answer only to the colleague being asked", async () => {
  mockFetch([ask()]);
  renderCard();

  await waitFor(() => {
    expect(screen.getByText(en["person.intro.answerAction"])).toBeTruthy();
  });
});

// The admit case above proves the button can appear at all, so this one proves
// it is the VIEWER that decides — without it, a card that never rendered the
// button would pass this test for the wrong reason.
it("shows the requester the state without an answer to give", async () => {
  mockFetch([ask()], "u-requester");
  renderCard();

  await waitFor(() => {
    expect(screen.getByText(en["person.intro.stateRequested"])).toBeTruthy();
  });
  expect(screen.queryByText(en["person.intro.answerAction"])).toBeNull();
});

it("renders nothing at all when nobody has asked", async () => {
  mockFetch([]);
  const { container } = renderCard();

  await waitFor(() => {
    expect(container.querySelector(".pn-asks")).toBeNull();
  });
  expect(screen.queryByText(en["person.intro.asksTitle"])).toBeNull();
});
