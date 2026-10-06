/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { mountFilters } from "./filters.testkit";

afterEach(() => {
  vi.unstubAllGlobals();
});

// A request the fake server was never taught must fail where a suite can see
// it; an empty success would let a mistyped or unexpected read pass unnoticed.
describe("The filters fake server", () => {
  it.each([
    ["an address nobody registered", "/v1/deals"],
    ["an address below one list", "/v1/lists/l1/members"],
    ["an address below one view", "/v1/views/v1/history"],
  ])("answers %s as not found", async (_, path) => {
    mountFilters({ listsOn: true });
    const answer = await fetch(`http://localhost:3000${path}`);
    expect(answer.status).toBe(404);
    expect(answer.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await answer.json()).toMatchObject({ code: "not_found" });
  });

  it("still answers the catalog and one list's history", async () => {
    mountFilters({ listsOn: true });
    const catalog = await fetch("http://localhost:3000/v1/lists");
    const history = await fetch("http://localhost:3000/v1/lists/l1/history");
    expect(await catalog.json()).toMatchObject({ data: [] });
    expect(await history.json()).toMatchObject({ data: [] });
  });
});
