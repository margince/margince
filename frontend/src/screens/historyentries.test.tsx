/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
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
