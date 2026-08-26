/** @vitest-environment jsdom */

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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordHistory } from "./historyentries";

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const repriced = {
  id: "h1",
  actor_type: "human",
  actor_id: "human:u1",
  actor_name: "Demo Admin",
  action: "update",
  occurred_at: "2026-07-14T10:00:00Z",
  summary: "Demo Admin updated the record",
  before: { amount_minor: 2500000 },
  after: { amount_minor: 4150000 },
};

function servingOnePage(entries: readonly unknown[]) {
  return vi.fn(async () =>
    jsonResponse({ data: entries, page: { next_cursor: null } }),
  );
}

describe("RecordHistory field detail", () => {
  // The detail used to live behind a sub-tab nobody lands on. What a change
  // DID is the reason a reader opens this list at all.
  it("shows what one entry changed, on the list the reader lands on", async () => {
    vi.stubGlobal("fetch", servingOnePage([repriced]));
    render(<RecordHistory kind="deal" id="d1" currency="EUR" />);
    expect(await screen.findByText("Value")).toBeTruthy();
    expect(screen.getByText(/25,000/)).toBeTruthy();
    expect(screen.getByText(/41,500/)).toBeTruthy();
    expect(screen.queryByText("2500000")).toBeNull();
  });

  it("says nothing extra for an entry that carries no images", async () => {
    vi.stubGlobal(
      "fetch",
      servingOnePage([{ ...repriced, before: null, after: null }]),
    );
    render(<RecordHistory kind="deal" id="d1" currency="EUR" />);
    expect(
      await screen.findByText("Demo Admin updated the record"),
    ).toBeTruthy();
    expect(screen.queryByText("Value")).toBeNull();
  });
});

// A change one field wide, restorable, with a version to pin the write.
const restorable = {
  ...repriced,
  undoable: { undoable: true, reason: null, detail: null },
};
const twoFields = {
  ...restorable,
  before: { amount_minor: 2500000, name: "Globex" },
  after: { amount_minor: 4150000, name: "Globex Renewal" },
};
const RESTORE = { version: 7, onRestored: () => {} };

function restoreCalls(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls
    .map(([input]) => (input instanceof Request ? input : null))
    .filter((request) => request !== null)
    .filter((request) => request.url.includes("/restore"));
}

describe("putting one change back", () => {
  it("sends the restore pinned to the version it was read at", async () => {
    const fetchMock = servingOnePage([restorable]);
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(
      <RecordHistory kind="deal" id="d1" currency="EUR" restore={RESTORE} />,
    );

    await user.click(await screen.findByRole("button", { name: /put back/i }));

    await waitFor(() => expect(restoreCalls(fetchMock)).toHaveLength(1));
    const [request] = restoreCalls(fetchMock);
    expect(request.method).toBe("POST");
    expect(request.url).toContain("/records/deal/d1/history/h1/restore");
    expect(request.headers.get("If-Match")).toBe("7");
  });

  // A greyed control that says nothing is the shape this feature exists to
  // remove: the reason is the information.
  it("states the reason a refused change cannot be put back", async () => {
    vi.stubGlobal(
      "fetch",
      servingOnePage([
        {
          ...repriced,
          undoable: {
            undoable: false,
            reason: "superseded",
            detail: "amount_minor",
          },
        },
      ]),
    );
    render(
      <RecordHistory kind="deal" id="d1" currency="EUR" restore={RESTORE} />,
    );

    const button = await screen.findByRole("button", { name: /put back/i });
    expect(button.hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(/changed these fields since/i)).toBeTruthy();
  });

  // The same words on both sides of the press. A refusal discovered at press
  // time and one shown up front must not read as two different products.
  it("answers a refused press in the words the refused button uses", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/restore")) {
        return jsonResponse(
          { type: "about:blank", status: 409, code: "already_undone" },
          409,
        );
      }
      return jsonResponse({
        data: [restorable],
        page: { next_cursor: null },
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(
      <RecordHistory kind="deal" id="d1" currency="EUR" restore={RESTORE} />,
    );

    await user.click(await screen.findByRole("button", { name: /put back/i }));

    expect(await screen.findByText(/has already been put back/i)).toBeTruthy();
  });

  // The record moved, not the change. The reader is told so and the history is
  // re-read, rather than being handed a failure they cannot act on.
  it("says the record moved and re-reads it on a version skew", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/restore")) {
        return jsonResponse(
          { type: "about:blank", status: 409, code: "version_skew" },
          409,
        );
      }
      return jsonResponse({
        data: [restorable],
        page: { next_cursor: null },
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    const reread = vi.fn();
    const user = userEvent.setup();
    render(
      <RecordHistory
        kind="deal"
        id="d1"
        currency="EUR"
        restore={{ version: 7, onRestored: reread }}
      />,
    );

    await user.click(await screen.findByRole("button", { name: /put back/i }));

    expect(await screen.findByText(/record moved/i)).toBeTruthy();
    expect(reread).toHaveBeenCalled();
  });

  // A write on somebody else's data, touching more than one field, is worth a
  // sentence naming what moves before it lands.
  it("names the fields before putting more than one of them back", async () => {
    const fetchMock = servingOnePage([twoFields]);
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(
      <RecordHistory kind="deal" id="d1" currency="EUR" restore={RESTORE} />,
    );

    await user.click(await screen.findByRole("button", { name: /put back/i }));

    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByText(/2 fields/i)).toBeTruthy();
    expect(within(dialog).getByText("Value")).toBeTruthy();
    expect(within(dialog).getByText("Name")).toBeTruthy();
    expect(restoreCalls(fetchMock)).toHaveLength(0);

    await user.click(within(dialog).getByRole("button", { name: /put back/i }));
    await waitFor(() => expect(restoreCalls(fetchMock)).toHaveLength(1));
  });

  // Nothing to pin the write against is nothing to offer: a restore with no
  // precondition is last-write-wins, which this control may not choose.
  it("offers no verb on a record read back without a version", async () => {
    vi.stubGlobal("fetch", servingOnePage([restorable]));
    render(
      <RecordHistory
        kind="deal"
        id="d1"
        currency="EUR"
        restore={{ version: undefined, onRestored: () => {} }}
      />,
    );

    expect(await screen.findByText("Value")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /put back/i })).toBeNull();
  });
});
