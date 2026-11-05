// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UsersAdminCard } from "./users-admin";
import { backend, jsonResponse, ROSTER, render } from "./users-admin.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

// The roster the server pages: the first page carries a cursor, the second
// is the rest.
function pagedRoster() {
  const routed = backend([]);
  const reads: string[] = [];
  const answer = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url);
    if (url.pathname.endsWith("/v1/users") && req.method === "GET") {
      reads.push(url.search);
      const later = url.searchParams.get("cursor") === "page-2";
      return jsonResponse({
        data: later
          ? [{ ...ROSTER.data[0], id: "u-late", display_name: "Lena Late" }]
          : ROSTER.data,
        page: later
          ? { next_cursor: null, has_more: false }
          : { next_cursor: "page-2", has_more: true },
      });
    }
    return routed(input, init);
  });
  return { answer, reads };
}

describe("the members list past its first page", () => {
  it("reaches the members the first page did not carry", async () => {
    const { answer, reads } = pagedRoster();
    vi.stubGlobal("fetch", answer);
    render(<UsersAdminCard />);

    await waitFor(() => expect(screen.getByText("Lena Late")).toBeTruthy());
    expect(reads.at(-1)).toContain("cursor=page-2");
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    vi.useFakeTimers({ shouldAdvanceTime: true });
    await user.type(
      screen.getByRole("searchbox", { name: "Search members" }),
      "Lena",
    );
    expect(screen.getByText("Lena Late")).toBeTruthy();
    expect(screen.queryByText("Ada Active")).toBeNull();
    await user.clear(screen.getByRole("searchbox", { name: "Search members" }));
    expect(screen.getByText("Ada Active")).toBeTruthy();
  });
});
