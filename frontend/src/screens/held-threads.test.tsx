// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { HeldThreadsCard } from "./held-threads";

// What this card must never do is state a fact it does not have. A thread whose
// opening message was erased has no subject; a thread nobody has judged has no
// kind; and a release that opened nothing must say so rather than looking as
// though it worked.

type HeldThread = components["schemas"]["HeldThread"];

const PENDING: HeldThread = {
  thread_key: "t-pending",
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
};

// And the refusal, as the server actually answers it: the message EXISTS —
// `has_message` is read without the content gate — and neither its subject nor
// its id crossed the clause.
const WITHHELD: HeldThread = {
  ...JUDGED,
  thread_key: "t-withheld",
  subject: undefined,
  activity_id: undefined,
};

function renderCard(rows: HeldThread[], share?: Record<string, unknown>) {
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
        return new Response(JSON.stringify({ data: rows }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
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
              can_change: false,
              change_mode: "message",
              held_by_others: false,
            },
            can_reply: false,
            can_relink: false,
            version: 1,
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
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
        <HeldThreadsCard />
      </LocaleProvider>
    </QueryClientProvider>
  );
  return render(ui);
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("held threads", () => {
  it("says a pending thread is waiting rather than leaving the reason blank", async () => {
    // During an outage every row is pending. A blank reason column would read
    // as a classifier that judged nothing rather than one that has not
    // answered, and the attempts count is what tells a stalled model from a
    // slow one.
    renderCard([PENDING]);
    expect(
      await screen.findByText(/waiting on a verdict/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/asked 3 time/i)).toBeInTheDocument();
  });

  it("names why a judged thread is held", async () => {
    renderCard([JUDGED]);
    expect(await screen.findByText("Legal")).toBeInTheDocument();
    expect(screen.queryByText(/waiting on a verdict/i)).not.toBeInTheDocument();
  });

  it("says the opening message is gone rather than drawing an empty subject", async () => {
    // The verdict outlives its evidence on purpose — losing it would re-open a
    // thread a classifier already held — so this row is normal, not broken.
    renderCard([
      {
        ...JUDGED,
        has_message: false,
        subject: undefined,
        occurred_at: undefined,
      },
    ]);
    expect(
      await screen.findByText(/the message this began with is gone/i),
    ).toBeInTheDocument();
  });

  it("tells a message with no subject from no message at all", async () => {
    // Both draw an unnamed row, and collapsing them would tell a reader their
    // evidence was destroyed when it is sitting there unnamed. This one keeps
    // its id: the content clause admitted the message, and the sender simply
    // left the subject line empty.
    renderCard([{ ...JUDGED, subject: undefined }]);
    expect(await screen.findByText(/^no subject$/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/the message this began with is gone/i),
    ).not.toBeInTheDocument();
    // And not the withheld reading either: a blank subject is a fact about the
    // sender, "not shared with you" a fact about the reader.
    expect(screen.queryByText(/not shared with you/i)).not.toBeInTheDocument();
  });

  it("does not report a withheld message as one that was erased", async () => {
    // The three states are separate all the way down. A message this reader
    // may not open still EXISTS, so telling them the evidence they are holding
    // is gone would be false — and it is the release they would stop asking
    // for.
    renderCard([WITHHELD]);
    expect(await screen.findByText(/not shared with you/i)).toBeInTheDocument();
    expect(
      screen.queryByText(/the message this began with is gone/i),
    ).not.toBeInTheDocument();
  });

  it("refuses the release when there is no message left to share", async () => {
    // The release works on the seat's own messages on the thread, so with none
    // left it answers not-found. A control whose only outcome is an error is
    // worse than one that says why it cannot run.
    renderCard([{ ...JUDGED, has_message: false, subject: undefined }]);
    const button = await screen.findByRole("button", {
      name: /share with the team/i,
    });
    expect(button).toBeDisabled();
    expect(screen.getByText(/no message left to share/i)).toBeInTheDocument();
  });

  it("reports a release that opened nothing", async () => {
    // A message two mailboxes imported opens only when both owners release it.
    // Reporting the other holder is the difference between a control that looks
    // broken and one that says what happened.
    const user = userEvent.setup();
    renderCard([JUDGED], { shared: false, held_by_others: 1 });
    await user.click(
      await screen.findByRole("button", { name: /share with the team/i }),
    );
    expect(await screen.findByText(/still held/i)).toBeInTheDocument();
  });

  it("reports an empty list as nothing held rather than as a failure", async () => {
    renderCard([]);
    expect(
      await screen.findByText(/withholding nothing right now/i),
    ).toBeInTheDocument();
  });
});

// A held thread whose message this reader may read opens it. The server sends
// `activity_id` only for those — it reads the id off the gated join — so the
// presence of the field IS the permission.
it("opens the message a held thread is about", async () => {
  const openable: HeldThread = {
    ...JUDGED,
    thread_key: "t-openable",
    activity_id: "01a05500-0000-7000-8000-0000000000b1",
  };
  renderCard([openable]);

  const button = await screen.findByRole("button", {
    name: "Entwurf Aufhebungsvertrag",
  });
  expect(button.getAttribute("aria-haspopup")).toBe("dialog");
  // Pressing it asks for THIS message's presentation — the screen's half of
  // the job. What the drawer then draws is emaildetail's own contract.
  await userEvent.click(button);
  await waitFor(() => {
    const asked = (
      globalThis.fetch as ReturnType<typeof vi.fn>
    ).mock.calls.some((call) => {
      const [input] = call;
      const url = input instanceof Request ? input.url : String(input);
      return url.includes(
        "/activities/01a05500-0000-7000-8000-0000000000b1/email-presentation",
      );
    });
    expect(asked).toBe(true);
  });
});

// And a thread whose message it may NOT read says so and offers nothing.
//
// The server withholds the subject and the id together for a message outside
// the caller's audience, so the row names itself as withheld rather than
// printing words it was refused, and does not become a control — a button that
// opened nothing would be worse than the plain text it replaced.
it("says a thread is not the reader's without offering to open it", async () => {
  renderCard([WITHHELD]);

  expect(await screen.findByText(/not shared with you/i)).toBeTruthy();
  // The row is still LISTED: they are holding it and must be able to release
  // it. Only the message's words are refused.
  expect(screen.getByText("Legal")).toBeTruthy();
  expect(screen.queryByRole("button", { name: /not shared/i })).toBeNull();
});
