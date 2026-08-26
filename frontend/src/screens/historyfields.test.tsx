/** @vitest-environment jsdom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { FieldHistoryTimeline } from "./historyfields";

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
  id: "f1",
  entity_type: "deal",
  entity_id: "d1",
  field: "amount_minor",
  old_value: "2500000",
  new_value: "4150000",
  changed_at: "2026-07-14T10:00:00Z",
  actor_type: "human",
  actor_id: "human:u1",
};

function servingOnePage(entries: readonly unknown[]) {
  return vi.fn(async () =>
    jsonResponse({ data: entries, page: { next_cursor: null } }),
  );
}

describe("FieldHistoryTimeline money", () => {
  it("reads a repricing at the deal's own scale, never as its stored integer", async () => {
    vi.stubGlobal("fetch", servingOnePage([repriced]));
    render(<FieldHistoryTimeline kind="deal" id="d1" currency="EUR" />);
    expect(await screen.findByText(/25,000/)).toBeTruthy();
    expect(screen.getByText(/41,500/)).toBeTruthy();
    expect(screen.queryByText("2500000")).toBeNull();
  });

  // The scale is a property of the currency. A dong figure divided by a
  // hundred is a hundredth of what the record holds, which is the same
  // misreading one currency over.
  it("keeps every digit of a currency that has no minor unit", async () => {
    vi.stubGlobal("fetch", servingOnePage([repriced]));
    render(<FieldHistoryTimeline kind="deal" id="d1" currency="VND" />);
    expect(await screen.findByText(/2,500,000/)).toBeTruthy();
  });

  it("names the field rather than printing its column", async () => {
    vi.stubGlobal("fetch", servingOnePage([repriced]));
    render(<FieldHistoryTimeline kind="deal" id="d1" currency="EUR" />);
    expect(await screen.findByText("Value")).toBeTruthy();
    expect(screen.queryByText("amount_minor")).toBeNull();
  });
});
