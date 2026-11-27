/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DealsScreen } from "./deals";
import { deal, render, stubBackend } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

const MATCHING = 3412;

// The deals list as the server pages it: counted on a read without a cursor,
// and opened at `offset` for a page past the first read.
function pagedDeals() {
  const base = stubBackend([deal({})]);
  const dealReads: URL[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = new Request(input, init);
      const url = new URL(request.url, "https://test.local");
      if (!url.pathname.endsWith("/deals")) {
        return base(request);
      }
      dealReads.push(url);
      const start = Number(url.searchParams.get("offset") ?? 0);
      const end = Math.min(
        MATCHING,
        start + Number(url.searchParams.get("limit")),
      );
      const data = Array.from({ length: end - start }, (_, index) =>
        deal({ id: `d-${start + index}`, name: `Deal ${start + index + 1}` }),
      );
      return Response.json({
        data,
        page: {
          next_cursor: end < MATCHING ? `after-${end}` : null,
          has_more: end < MATCHING,
          total: url.searchParams.has("cursor") ? undefined : MATCHING,
        },
      });
    }),
  );
  return dealReads;
}

describe("the deals table's pager", () => {
  it("numbers the last page the server's count names", async () => {
    window.location.hash = "#/deals?view=table";
    pagedDeals();
    render(<DealsScreen />);

    expect(await screen.findByText("Deal 1")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Page 137" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/of 3,412 deals/)).toBeInTheDocument();
  });

  it("opens a pasted link to the last page in one read at its offset", async () => {
    window.location.hash = "#/deals?view=table&page=137";
    const dealReads = pagedDeals();
    render(<DealsScreen />);

    expect(await screen.findByText("Deal 3401")).toBeInTheDocument();
    expect(dealReads.map((url) => url.searchParams.get("offset"))).toEqual([
      null,
      "3400",
    ]);
    expect(dealReads.some((url) => url.searchParams.has("cursor"))).toBe(false);
    expect(screen.getByRole("button", { name: "Page 137" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });
});
