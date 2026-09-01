/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

type Call = { method: string; path: string };

function mockFetch(
  asks: unknown[],
  viewer: string | null = VIEWER,
  calls: Call[] = [],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      // openapi-fetch hands the shim a Request, so the path is on .url —
      // stringifying the object itself yields "[object Request]" and matches
      // nothing, which renders as a card that simply never loads.
      const path = typeof input === "string" ? input : (input as Request).url;
      const method =
        typeof input === "string"
          ? (init?.method ?? "GET")
          : (input as Request).method;
      // A write against one ask (complete/cancel/decision) shares the
      // "/intro-requests" substring with the list read, so it is matched
      // FIRST and recorded — the shape of the response back doesn't matter to
      // these tests, only that the right verb reached the right path.
      if (method !== "GET" && path.includes("/intro-requests/")) {
        calls.push({ method, path });
        return new Response(JSON.stringify(asks[0] ?? {}), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
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

// The outcome (mark introduced / name used) and withdraw actions on an
// answered ask, gated on the same actor rules the server's lifecycle table
// enforces.

it("offers either party the handshake once the colleague has accepted", async () => {
  mockFetch([ask({ status: "accepted" })]);
  renderCard();
  await waitFor(() => {
    expect(
      screen.getByText(en["person.intro.completeIntroducedAction"]),
    ).toBeTruthy();
  });

  cleanup();
  mockFetch([ask({ status: "accepted" })], "u-requester");
  renderCard();
  await waitFor(() => {
    expect(
      screen.getByText(en["person.intro.completeIntroducedAction"]),
    ).toBeTruthy();
  });
});

it("offers only the requester the name-drop, since only they can have used it", async () => {
  mockFetch([ask({ status: "name_drop_approved" })], "u-requester");
  renderCard();
  await waitFor(() => {
    expect(
      screen.getByText(en["person.intro.completeNameDroppedAction"]),
    ).toBeTruthy();
  });

  cleanup();
  mockFetch([ask({ status: "name_drop_approved" })]);
  renderCard();
  await waitFor(() => {
    expect(
      screen.getByText(en["person.intro.stateNameDropApproved"]),
    ).toBeTruthy();
  });
  expect(
    screen.queryByText(en["person.intro.completeNameDroppedAction"]),
  ).toBeNull();
});

it("lets only the requester withdraw a still-open ask, from any open status", async () => {
  for (const status of ["requested", "accepted", "name_drop_approved"]) {
    cleanup();
    mockFetch([ask({ status })], "u-requester");
    renderCard();
    await waitFor(() => {
      expect(screen.getByText(en["person.intro.withdrawAction"])).toBeTruthy();
    });
  }

  cleanup();
  // The introducer sees the same open ask and gets no withdraw button — it is
  // the requester's ask to take back, not theirs to refuse a second way.
  mockFetch([ask({ status: "accepted" })]);
  renderCard();
  await waitFor(() => {
    expect(
      screen.getByText(en["person.intro.completeIntroducedAction"]),
    ).toBeTruthy();
  });
  expect(screen.queryByText(en["person.intro.withdrawAction"])).toBeNull();
});

const SETTLED_STATE_LABEL = {
  declined: en["person.intro.stateDeclined"],
  introduced: en["person.intro.stateIntroduced"],
  name_dropped: en["person.intro.stateNameDropped"],
  expired: en["person.intro.stateExpired"],
} as const;

it("offers no withdraw once the ask is settled — declined, introduced, or expired", async () => {
  for (const status of Object.keys(
    SETTLED_STATE_LABEL,
  ) as (keyof typeof SETTLED_STATE_LABEL)[]) {
    cleanup();
    mockFetch([ask({ status })], "u-requester");
    renderCard();
    await waitFor(() => {
      expect(screen.getByText(SETTLED_STATE_LABEL[status])).toBeTruthy();
    });
    expect(screen.queryByText(en["person.intro.withdrawAction"])).toBeNull();
  }
});

it("withdraws the ask the Withdraw button names, sending it to the cancel endpoint", async () => {
  const calls: Call[] = [];
  mockFetch([ask({ status: "requested" })], "u-requester", calls);
  renderCard();
  const withdraw = await screen.findByText(en["person.intro.withdrawAction"]);

  const user = userEvent.setup();
  await user.click(withdraw);

  await waitFor(() => {
    expect(
      calls.some(
        (call) =>
          call.method === "POST" &&
          call.path.includes("/intro-requests/ir-1/cancel"),
      ),
    ).toBe(true);
  });
});

it("records the handshake the Mark introduced button names, sending it to the complete endpoint", async () => {
  const calls: Call[] = [];
  mockFetch([ask({ status: "accepted" })], VIEWER, calls);
  renderCard();
  const markIntroduced = await screen.findByText(
    en["person.intro.completeIntroducedAction"],
  );

  const user = userEvent.setup();
  await user.click(markIntroduced);

  await waitFor(() => {
    expect(
      calls.some(
        (call) =>
          call.method === "POST" &&
          call.path.includes("/intro-requests/ir-1/complete"),
      ),
    ).toBe(true);
  });
});

// The two mutations are shared across the whole card, not one per row, so a
// failed complete on one row must not read as failed on a DIFFERENT row the
// reader acts on next — each row has to check whose id the failing mutation
// actually named, not just whether something on the card is in an error
// state.
it("keeps a failed outcome on the row it belongs to, not the next row acted on", async () => {
  const rows = [
    ask({ id: "ir-1", status: "accepted", internal_reason: "row one" }),
    ask({ id: "ir-2", status: "requested", internal_reason: "row two" }),
  ];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === "string" ? input : (input as Request).url;
      if (url.includes("/ir-1/complete")) {
        return new Response(JSON.stringify({ detail: "conflict" }), {
          status: 409,
          headers: { "content-type": "application/problem+json" },
        });
      }
      if (url.includes("/ir-2/cancel")) {
        return new Response(JSON.stringify(rows[1]), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.includes("/intro-requests")) {
        return new Response(JSON.stringify({ data: rows }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      if (url.endsWith("/me")) {
        return new Response(
          JSON.stringify({ user: { id: "u-requester" }, capabilities: {} }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response("{}", {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  renderCard();
  const user = userEvent.setup();

  function rowFor(reason: string): HTMLElement {
    const li = screen.getByText(reason).closest("li");
    if (!(li instanceof HTMLElement)) {
      throw new Error(`no row found for "${reason}"`);
    }
    return li;
  }

  await screen.findByText("row one");
  await user.click(
    within(rowFor("row one")).getByText(
      en["person.intro.completeIntroducedAction"],
    ),
  );
  await waitFor(() => {
    expect(
      within(rowFor("row one")).getByText(en["person.intro.completeFailed"]),
    ).toBeTruthy();
  });

  await user.click(
    within(rowFor("row two")).getByText(en["person.intro.withdrawAction"]),
  );
  await waitFor(() => {
    expect(
      within(rowFor("row two")).queryByText(en["person.intro.completeFailed"]),
    ).toBeNull();
  });
  // ir-1's own failure is still exactly where it happened.
  expect(
    within(rowFor("row one")).getByText(en["person.intro.completeFailed"]),
  ).toBeTruthy();
});
