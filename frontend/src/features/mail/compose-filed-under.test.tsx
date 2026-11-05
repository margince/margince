// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { ComposeModal } from "./compose";

// The way back to the record the message files under.
//
// The composer opens from surfaces that are not the record. The day's queue and
// a worklist row both draft to a contact whose page is nowhere on screen.
//
// It knows which record it writes under, because the send files under it. It
// said so nowhere a reader could follow.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

function stubRoutes() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const key = url.pathname.replace(/^\/v1/, "");
      if (key === "/consent-purposes") return jsonResponse(PURPOSES);
      if (key === "/voice-profiles") return jsonResponse({ data: [] });
      if (key === "/contacts/c-1") {
        return jsonResponse({ id: "c-1", full_name: "Anna Weber" });
      }
      return jsonResponse({});
    }),
  );
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

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the record a draft files under", () => {
  it("offers it as a link the reader can follow to the record", async () => {
    stubRoutes();
    render(
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        recordAddress="anna.weber@example.com"
        open
        onClose={vi.fn()}
      />,
    );
    const link = await screen.findByRole("link", { name: /Anna Weber/ });
    expect(link.getAttribute("href")).toContain("c-1");
    // In place, not a new tab: the dock keeps the window mounted across the
    // navigation, so the draft outlives following this.
    expect(link.getAttribute("target")).toBeNull();
  });

  it("says nothing when the composer writes under no record", async () => {
    stubRoutes();
    render(
      <ComposeModal
        entityType="contact"
        entityId={undefined as unknown as string}
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByRole("textbox", { name: "Body" });
    expect(screen.queryByText("Filed under")).toBeNull();
  });
});
