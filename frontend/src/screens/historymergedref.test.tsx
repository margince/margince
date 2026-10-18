/** @vitest-environment happy-dom */

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { RecordHistory } from "./historyentries";

// What the reader of a merge survivor's History sees: the merge explained in
// words, and the folded-in record named when this reader may read it.

const FOLDED_IN = "0190a7f2-0000-7000-8000-0000000000a1";

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

const mergeEntry = {
  id: "h1",
  actor_type: "human",
  actor_id: "human:u1",
  actor_name: "Demo Admin",
  action: "merge",
  occurred_at: "2026-07-14T10:00:00Z",
  summary: "Demo Admin merged the record",
  before: { merged_from_id: null, title: null },
  after: {
    merged_from_id: FOLDED_IN,
    title: "Chief Merge Officer",
    relinked_emails: 2,
    relinked_phones: 0,
    relinked_relationships: 1,
    relinked_activity_links: 3,
  },
};

function serving(contact: Response) {
  return vi.fn(async (input: Request | string | URL) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes("/history")) {
      return jsonResponse({ data: [mergeEntry], page: { next_cursor: null } });
    }
    return contact.clone();
  });
}

describe("a merge on the survivor's History", () => {
  it("names the record folded in and the counts in words", async () => {
    vi.stubGlobal(
      "fetch",
      serving(jsonResponse({ id: FOLDED_IN, full_name: "Ann Smith" })),
    );
    render(<RecordHistory kind="contact" id="c1" currency={null} />);

    expect(await screen.findByText("Merged in from")).toBeTruthy();
    expect(await screen.findByText("Ann Smith")).toBeTruthy();
    expect(screen.getByText("Email addresses moved over")).toBeTruthy();
    expect(screen.getByText("Relationships moved over")).toBeTruthy();
    expect(screen.getByText("Chief Merge Officer")).toBeTruthy();
    expect(screen.queryByText(FOLDED_IN)).toBeNull();
    expect(screen.queryByText("merged from id")).toBeNull();
  });

  it("says another record when this reader may not read the one folded in", async () => {
    vi.stubGlobal("fetch", serving(jsonResponse({ title: "Not found" }, 404)));
    render(<RecordHistory kind="contact" id="c1" currency={null} />);

    expect(await screen.findByText("another record")).toBeTruthy();
    expect(screen.queryByText(FOLDED_IN)).toBeNull();
  });
});
