/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useLeadingCurrencies } from "./leadingcurrencies";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Answer = { body: unknown; status?: number } | "pending";

// The two reads the hook makes. Every other path is a test bug, so it fails loudly.
function stubReads(settings: Answer, sheet: Answer) {
  const answer = (a: Answer) =>
    a === "pending"
      ? new Promise<Response>(() => {})
      : Promise.resolve(jsonResponse(a.body, a.status));
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      const path = new URL(url, "https://test.local").pathname;
      if (path.endsWith("/installation/settings")) return answer(settings);
      if (path.endsWith("/fx-rates/currencies")) return answer(sheet);
      throw new Error(`unexpected request ${path}`);
    }),
  );
}

function renderLeading() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  return renderHook(() => useLeadingCurrencies(), { wrapper });
}

const NOK = { body: { base_currency: "NOK" } };
const failed = { body: { title: "boom" }, status: 500 };

describe("useLeadingCurrencies", () => {
  it("leads with the base, then the sheet's codes without the base twice", async () => {
    stubReads(NOK, {
      body: { base_currency: "NOK", data: ["NOK", "SEK", "DKK"] },
    });
    const { result } = renderLeading();
    await waitFor(() => expect(result.current).toEqual(["NOK", "SEK", "DKK"]));
  });

  it("answers the base alone while the sheet is still on its way", async () => {
    stubReads(NOK, "pending");
    const { result } = renderLeading();
    await waitFor(() => expect(result.current).toEqual(["NOK"]));
  });

  it("answers the base alone when the sheet read fails", async () => {
    stubReads(NOK, failed);
    const { result } = renderLeading();
    await waitFor(() => expect(result.current).toEqual(["NOK"]));
  });

  it("takes the base from the sheet read for a role refused the settings", async () => {
    stubReads(
      { body: { title: "forbidden" }, status: 403 },
      { body: { base_currency: "NOK", data: ["SEK"] } },
    );
    const { result } = renderLeading();
    await waitFor(() => expect(result.current).toEqual(["NOK", "SEK"]));
  });

  it("falls back to the four common codes when neither read names a base", async () => {
    stubReads(failed, failed);
    const { result } = renderLeading();
    await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalledTimes(2));
    expect(result.current).toEqual(["EUR", "USD", "GBP", "CHF"]);
  });
});
