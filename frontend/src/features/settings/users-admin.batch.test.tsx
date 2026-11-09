// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MEMBER_BATCH, UsersAdminCard } from "./users-admin";
import { backend, jsonResponse, ROSTER, render } from "./users-admin.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const SIZE = 450;

// A roster of 450 made-up members, answered in one page.
function largeRoster() {
  const template = ROSTER.data[0];
  const data = Array.from({ length: SIZE }, (_, at) => {
    const n = String(at).padStart(3, "0");
    return {
      ...template,
      id: `u-roster-${n}`,
      display_name: `Roster Member ${n}`,
      email: `roster-${n}@example.test`,
    };
  });
  const routed = backend([]);
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    const url = new URL(req.url);
    if (url.pathname.endsWith("/v1/users") && req.method === "GET") {
      return jsonResponse({
        data,
        page: { next_cursor: null, has_more: false },
      });
    }
    return routed(input, init);
  });
}

const drawnNames = () => screen.queryAllByText(/^Roster Member \d{3}$/).length;

// The whole roster is read, for the checks that need every member, but its
// rows are drawn a batch at a time. The caption still counts every member.
describe("a large members list", () => {
  it("draws the first batch, then the next one on request", async () => {
    // Without an observer the sentinel offers its button, which a test can press.
    vi.stubGlobal("IntersectionObserver", undefined);
    vi.stubGlobal("fetch", largeRoster());
    render(<UsersAdminCard />);

    await waitFor(() => expect(drawnNames()).toBe(MEMBER_BATCH));
    expect(screen.getByText(`${SIZE} of ${SIZE} members`)).toBeTruthy();

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Load more" }));
    await waitFor(() => expect(drawnNames()).toBe(2 * MEMBER_BATCH));
  });

  it("finds a member past the first batch by searching", async () => {
    vi.stubGlobal("fetch", largeRoster());
    render(<UsersAdminCard />);
    await waitFor(() => expect(drawnNames()).toBe(MEMBER_BATCH));

    const user = userEvent.setup();
    await user.type(
      screen.getByRole("searchbox", { name: "Search members" }),
      "Roster Member 321",
    );
    await waitFor(() => expect(drawnNames()).toBe(1));
    expect(screen.getByText("Roster Member 321")).toBeTruthy();
    expect(screen.getByText(`1 of ${SIZE} members`)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });
});

// Clearing a search starts again at the first batch, even when the empty
// search had loaded more before.
//
// Mutation: keep the batch count for a search string seen before, and the
// cleared list redraws 200 rows at once.
describe("a search changed and cleared", () => {
  it("starts again at the first batch", async () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    vi.stubGlobal("fetch", largeRoster());
    render(<UsersAdminCard />);
    await waitFor(() => expect(drawnNames()).toBe(MEMBER_BATCH));
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Load more" }));
    await waitFor(() => expect(drawnNames()).toBe(2 * MEMBER_BATCH));

    const search = screen.getByRole("searchbox", { name: "Search members" });
    await user.type(search, "Roster Member 3");
    await waitFor(() => expect(drawnNames()).toBeLessThan(MEMBER_BATCH + 1));
    await user.clear(search);
    await waitFor(() => expect(drawnNames()).toBe(MEMBER_BATCH));
  });
});
