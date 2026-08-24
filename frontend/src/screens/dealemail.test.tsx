// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DealEmailAside } from "./dealemail";

// The box offers one of two things and never nothing, and WHICH one it offers
// is the whole feature: a rep who is owed an answer must be offered the reply,
// and a rep whose buyer has never written must still be offered a mail rather
// than an empty panel. The label is the only thing that tells them apart on
// screen, so it is what these tests read.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL = "01a03000-0000-7000-8000-000000000001";
const MAIL = "01a03000-0000-7000-8000-0000000000aa";

function serveCard(replyTo: string | null) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/status")) {
        return new Response(
          JSON.stringify({
            deal_id: DEAL,
            story: { sentences: [] },
            reply_to: replyTo,
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "deterministic",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function renderBox() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <DealEmailAside dealId={DEAL} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the deal's email box", () => {
  it("offers the reply when the buyer wrote and nobody has answered", async () => {
    serveCard(MAIL);
    renderBox();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Draft the reply" }),
      ).toBeTruthy(),
    );
  });

  it("offers a fresh mail when there is nothing to answer", async () => {
    serveCard(null);
    renderBox();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Send an email" }),
      ).toBeTruthy(),
    );
    expect(
      screen.queryByRole("button", { name: "Draft the reply" }),
    ).toBeNull();
  });

  // The read is in flight on the first paint, and a box that rendered nothing
  // until it answered would flicker an empty column on every deal open. It
  // starts on the fresh-mail side, which is the safe half: a rep who meant to
  // reply sees the thread missing, where the other way round would file a new
  // message onto a thread they never chose.
  it("offers a fresh mail before the read has answered", () => {
    serveCard(MAIL);
    renderBox();
    expect(screen.getByRole("button", { name: "Send an email" })).toBeTruthy();
  });
});
