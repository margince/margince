// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { heldThreadList } from "../../testing/heldthreads.fixtures";
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { linkedInReachFixture } from "../../testing/reachfixture";
import { ConnectionOutcomesCard } from "./connection-outcomes";

// What this card must never do is state a fact it does not have. A thread whose
// opening message was erased has no subject; a thread nobody has judged has no
// kind; and a release that opened nothing must say so rather than looking as
// though it worked.

type HeldThread = components["schemas"]["HeldThread"];

const PENDING: HeldThread = {
  thread_key: "t-pending",
  activity_id: "01a05500-0000-7000-8000-0000000000b2",
  status: "pending",
  pending: true,
  attempts: 3,
  has_message: true,
  subject: "Angebot Q4",
  occurred_at: "2026-08-30T09:12:00Z",
};

// A message this reader may read. The subject and the id come through the SAME
// content clause on the server, so a row carrying one carries the other — a
// fixture with a subject and no id would describe a record the server cannot
// send, and every component judged against it would be judged against a shape
// that does not exist.
const JUDGED: HeldThread = {
  thread_key: "t-legal",
  status: "held",
  pending: false,
  attempts: 1,
  has_message: true,
  kind: "legal",
  subject: "Entwurf Aufhebungsvertrag",
  activity_id: "01a05500-0000-7000-8000-0000000000b1",
  occurred_at: "2026-08-29T15:40:00Z",
  participants: [],
  participant_count: 0,
};

// And the refusal, as the server actually answers it: the message EXISTS —
// `has_message` is read without the content gate — and neither its subject nor
// its id crossed the clause.
const WITHHELD: HeldThread = {
  ...JUDGED,
  thread_key: "t-withheld",
  subject: undefined,
  activity_id: undefined,
  participants: undefined,
  participant_count: undefined,
};

type Clock = components["schemas"]["CaptureVerdictClock"];

// The list opens from the outcomes card's held-threads row, in a drawer.
async function renderCard(
  rows: HeldThread[],
  share?: Record<string, unknown>,
  threadVerdict?: Clock,
  browse = true,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      if (key === "GET /me") {
        return new Response(JSON.stringify(meFixture({})), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (key === "GET /capture/held-threads") {
        return new Response(
          JSON.stringify(
            heldThreadList(rows, { thread_verdict: threadVerdict }),
          ),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        );
      }
      // The drawer's own read. Served here so opening a thread does not throw
      // on a body with no access block; what the drawer draws from it is
      // emaildetail's contract, tested there.
      if (key.endsWith("/email-presentation")) {
        return new Response(
          JSON.stringify({
            id: "01a05500-0000-7000-8000-0000000000b1",
            lifecycle: "delivered",
            occurred_at: "2026-08-29T15:40:00Z",
            summary: {
              activity_id: "01a05500-0000-7000-8000-0000000000b1",
              occurred_at: "2026-08-29T15:40:00Z",
              version: 1,
              subject: "Entwurf Aufhebungsvertrag",
              display_status: "participants",
              move: "none",
              attachment_count: 0,
            },
            body: "…",
            thread_key: "t-legal",
            from: [],
            to: [],
            cc: [],
            bcc: [],
            bcc_withheld: false,
            attachments: [],
            links: [],
            thread: { members: [], next_cursor: null },
            access: {
              content_state: "available",
              audience: "participants",
              selected_members: [],
              display_status: "participants",
              can_change: true,
              change_mode: "thread_contribution",
              held_by_others: false,
            },
            can_reply: false,
            can_forward: true,
            can_relink: false,
            version: 1,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (
        key === "GET /capture/senders" ||
        key === "GET /capture/contacts-awaiting-decision"
      ) {
        return new Response(JSON.stringify({ data: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (key === "GET /me/linkedin-reach") {
        return new Response(JSON.stringify(linkedInReachFixture()), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      if (key.startsWith("POST /activities/threads/")) {
        return new Response(JSON.stringify(share ?? { shared: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("{}", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui: ReactNode = (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConnectionOutcomesCard />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const user = userEvent.setup();
  render(ui);
  if (rows.length > 0) {
    await user.click(
      await screen.findByRole("button", {
        name: "View Mail not shared with the team",
      }),
    );
    await screen.findByRole("dialog", {
      name: "Mail not shared with the team",
    });
    if (browse)
      await user.click(
        screen.getByRole("button", { name: "Browse recent conversations" }),
      );
  }
  return user;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("mail sharing corrections", () => {
  it("opens with search and no review backlog", async () => {
    await renderCard([JUDGED], undefined, undefined, false);
    expect(
      screen.getByRole("searchbox", { name: en["heldThreads.search"] }),
    ).toBeInTheDocument();
    expect(screen.queryByText(JUDGED.subject ?? "")).toBeNull();
    expect(
      screen.queryByRole("button", { name: /share with the team/i }),
    ).toBeNull();
    expect(screen.getAllByText(en["heldThreads.sub"]).length).toBeGreaterThan(
      0,
    );
  });
  it("browses reasons without offering a share action on each row", async () => {
    await renderCard([PENDING, JUDGED]);
    expect(await screen.findByText("Legal")).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: "Preview and change sharing" }),
    ).toHaveLength(2);
    expect(
      screen.queryByRole("button", { name: /share with the team/i }),
    ).toBeNull();
  });
  it("omits retained holds without a readable message", async () => {
    await renderCard([
      WITHHELD,
      { ...JUDGED, has_message: false, activity_id: undefined },
    ]);
    expect(screen.queryByText("Legal")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Preview and change sharing" }),
    ).toBeNull();
  });
  it("opens the selected message for preview and its existing access editor", async () => {
    const user = await renderCard([JUDGED]);
    await user.click(
      screen.getByRole("button", { name: "Preview and change sharing" }),
    );
    await waitFor(() =>
      expect(
        (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.some(
          ([input]) =>
            String(input instanceof Request ? input.url : input).includes(
              "/email-presentation",
            ),
        ),
      ).toBe(true),
    );
  });
  it("shows other owners still holding the conversation after sharing from its preview", async () => {
    const user = await renderCard([JUDGED], {
      messages: 1,
      shared: false,
      held_by_others: 2,
      activity_ids: [],
    });
    await user.click(
      screen.getByRole("button", { name: "Preview and change sharing" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Share with the company" }),
    );
    expect(
      await screen.findByText("2 other seats have not shared this thread"),
    ).toBeInTheDocument();
  });
  it("keeps ordinary classification cadence out of the correction drawer", async () => {
    await renderCard([PENDING], undefined, {
      every_seconds: 600,
      running: false,
      queued: false,
      next_pass_at: "2026-08-30T09:20:00Z",
    });
    expect(screen.queryByTestId("verdict-pass-threads")).toBeNull();
  });
});
