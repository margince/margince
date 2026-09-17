/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { meFixture } from "./mefixture";
import { NotificationBell } from "./notificationbell";

// The chrome's notification centre: the count a reader sees without opening
// anything, and the panel behind it.
//
// No grant fixture appears below on purpose — the store is principal-scoped, so
// a reader only ever sees their own notices and there is no seat that could be
// refused the panel.

type Notice = {
  id: string;
  kind: string;
  subject: string;
  body?: string;
  created_at: string;
  read_at?: string;
  target?: { type: string; id: string };
  origin?: {
    event_id: string;
    actor_type: string;
    actor_id: string;
    occurred_at: string;
  };
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function notice(
  id: string,
  subject: string,
  extra: Partial<Notice> = {},
): Notice {
  return {
    id,
    kind: "automation_failed",
    subject,
    created_at: "2026-09-15T09:00:00Z",
    ...extra,
  };
}

// backendFor answers the centre with the given notices and settles them the way
// the server does — the per-notice read and the bulk read both stamp `read_at`,
// and `unread_count` is recomputed from what is left, which is what the badge
// clearing depends on.
//
// The bulk settle answers 204 with NO BODY on purpose: the server's own count
// also covers notices the centre never shows, so no number reaches the UI and
// the screen re-asks instead.
function backendFor(notices: Notice[]) {
  let state = notices;
  const calls: string[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({}));
      }
      if (req.url.includes("/notices/read-all")) {
        calls.push("read-all");
        state = state.map((row) => ({
          ...row,
          read_at: row.read_at ?? "2026-09-15T10:00:00Z",
        }));
        return new Response(null, { status: 204 });
      }
      const settled = /\/notices\/([^/]+)\/read$/.exec(req.url);
      if (settled) {
        calls.push(`read ${settled[1]}`);
        state = state.map((row) =>
          row.id === settled[1]
            ? { ...row, read_at: "2026-09-15T10:00:00Z" }
            : row,
        );
        return new Response(null, { status: 204 });
      }
      if (req.url.includes("/notices")) {
        return jsonResponse({
          items: state,
          unread_count: state.filter((row) => !row.read_at).length,
        });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, calls: () => calls };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("NotificationBell", () => {
  it("carries how many notices are waiting", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "An automation could not run"),
        notice("n2", "A lead is past its deadline"),
      ]).fetchMock,
    );
    render(<NotificationBell />);

    const bell = await screen.findByRole("button", { name: /2 waiting/i });
    expect(within(bell).getByText("2")).not.toBeNull();
  });

  // A ZERO IS NOT A COUNT WORTH DRAWING. A badge reading "0" is a mark the eye
  // stops on to learn there is nothing, which is the one thing an unmarked bell
  // already says.
  it("draws no count when nothing is waiting", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "An automation could not run", {
          read_at: "2026-09-15T09:30:00Z",
        }),
      ]).fetchMock,
    );
    render(<NotificationBell />);

    const bell = await screen.findByRole("button", { name: /notifications/i });
    await waitFor(() =>
      expect(bell.getAttribute("aria-expanded")).toBe("false"),
    );
    expect(within(bell).queryByText("0")).toBeNull();
  });

  it("opens the centre on the bell and lists the newest notice first", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "A lead is past its deadline"),
        notice("n2", "An automation could not run", {
          created_at: "2026-09-14T09:00:00Z",
        }),
      ]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));

    const items = await screen.findAllByRole("listitem");
    expect(items[0]?.textContent).toContain("A lead is past its deadline");
    expect(items[1]?.textContent).toContain("An automation could not run");
  });

  // The chrome's one dismissal, so the centre closes the way every other
  // popover in the strip does.
  it("closes again on Escape", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([notice("n1", "A lead is past its deadline")]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));
    expect(
      await screen.findByText("A lead is past its deadline"),
    ).not.toBeNull();

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByText("A lead is past its deadline")).toBeNull(),
    );
  });

  // A notice already answered still belongs in the centre — it is history — and
  // has to read as answered rather than as one more thing waiting.
  it("tells a settled notice from one still waiting", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "A lead is past its deadline"),
        notice("n2", "An automation could not run", {
          created_at: "2026-09-14T09:00:00Z",
          read_at: "2026-09-14T10:00:00Z",
        }),
      ]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));

    const items = await screen.findAllByRole("listitem");
    expect(within(items[0] as HTMLElement).getByText("New")).not.toBeNull();
    expect(within(items[1] as HTMLElement).queryByText("New")).toBeNull();
  });

  it("settles what the reader could see, and the count goes with it", async () => {
    const backend = backendFor([
      notice("n1", "A lead is past its deadline"),
      notice("n2", "An automation could not run"),
    ]);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));
    await user.click(
      await screen.findByRole("button", { name: /mark all read/i }),
    );

    await waitFor(() => expect(backend.calls()).toEqual(["read-all"]));
    // The badge clears from the server's own re-answer rather than from a
    // number this screen kept: the bulk settle covers notices the centre never
    // shows, so its count is not one the UI may quote.
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /waiting/i })).toBeNull(),
    );
  });

  it("settles one notice on its own", async () => {
    const backend = backendFor([
      notice("n1", "A lead is past its deadline"),
      notice("n2", "An automation could not run"),
    ]);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));
    const items = await screen.findAllByRole("listitem");
    await user.click(
      within(items[0] as HTMLElement).getByRole("button", {
        name: /mark read/i,
      }),
    );

    // The id on the wire is the row's, not the first unread one the handler
    // could have found for itself.
    await waitFor(() => expect(backend.calls()).toEqual(["read n1"]));
    await waitFor(() =>
      expect(
        within(screen.getAllByRole("listitem")[0] as HTMLElement).queryByText(
          "New",
        ),
      ).toBeNull(),
    );
  });

  it("links a notice about a record to that record", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "A lead is past its deadline", {
          target: { type: "lead", id: "11111111-1111-4111-8111-111111111111" },
        }),
      ]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));

    const link = await screen.findByRole("link", {
      name: "A lead is past its deadline",
    });
    expect(link.getAttribute("href")).toBe(
      "#/leads/11111111-1111-4111-8111-111111111111",
    );
  });

  // A record KIND this app has no page for is named and not linked. Offering a
  // link into nothing is worse than offering none.
  it("names a notice it cannot route to, without linking it", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "A mailbox stopped answering", {
          target: { type: "activity", id: "a1" },
        }),
      ]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));

    expect(
      await screen.findByText("A mailbox stopped answering"),
    ).not.toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });

  // INDIGO IS A CLAIM ABOUT PROVENANCE, so it is drawn for the one notice an
  // agent authored and for no other row.
  it("says which notice an agent authored, and marks no other", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([
        notice("n1", "A draft is ready for you", {
          origin: {
            event_id: "22222222-2222-4222-8222-222222222222",
            actor_type: "agent",
            actor_id: "agent:drafter",
            occurred_at: "2026-09-15T08:59:00Z",
          },
        }),
        notice("n2", "An automation could not run", {
          created_at: "2026-09-14T09:00:00Z",
          origin: {
            event_id: "33333333-3333-4333-8333-333333333333",
            actor_type: "system",
            actor_id: "system:automations",
            occurred_at: "2026-09-14T08:59:00Z",
          },
        }),
      ]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(await screen.findByRole("button", { name: /waiting/i }));

    const items = await screen.findAllByRole("listitem");
    expect(
      within(items[0] as HTMLElement).getByText("By an agent"),
    ).not.toBeNull();
    expect(
      within(items[1] as HTMLElement).queryByText("By an agent"),
    ).toBeNull();
  });

  it("says so when nothing has been raised at all", async () => {
    vi.stubGlobal("fetch", backendFor([]).fetchMock);
    const user = userEvent.setup();
    render(<NotificationBell />);

    await user.click(
      await screen.findByRole("button", { name: /notifications/i }),
    );

    expect(await screen.findByText(/Nothing has come in/i)).not.toBeNull();
  });

  it("reads the centre in the reader's own language", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor([notice("n1", "A lead is past its deadline")]).fetchMock,
    );
    const user = userEvent.setup();
    render(<NotificationBell />, "de");

    await user.click(await screen.findByRole("button", { name: /wartet/i }));

    expect(
      await screen.findByRole("button", {
        name: /Alle als gelesen markieren/i,
      }),
    ).not.toBeNull();
  });
});
