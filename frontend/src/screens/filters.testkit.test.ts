/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { filterView, mountFilters } from "./filters.testkit";
import { LIVE_ID, liveList } from "./lists.fixtures";

afterEach(() => {
  vi.unstubAllGlobals();
});

const AT = "http://localhost:3000/v1";

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
    const catalog = await fetch(`${AT}/lists`);
    const history = await fetch(`${AT}/lists/l1/history`);
    expect(await catalog.json()).toMatchObject({ data: [] });
    expect(await history.json()).toMatchObject({ data: [] });
  });

  it("answers a stored list's visit with a whole visit body, and makes no list of it", async () => {
    mountFilters({ listsOn: true, lists: [liveList] });
    const visit = await fetch(`${AT}/lists/${LIVE_ID}/visit`, {
      method: "POST",
    });
    const catalog = await fetch(`${AT}/lists`);

    expect(visit.status).toBe(200);
    expect(await visit.json()).toEqual({
      list_id: LIVE_ID,
      visited_at: expect.any(String),
      previous_visit_at: expect.any(String),
    });
    expect((await catalog.json()).data).toEqual([liveList]);
  });

  it("refuses a view written without If-Match as it refuses a stale one", async () => {
    mountFilters({
      views: [filterView("v1", "Berliners", "contacts", { and: [] })],
    });
    const rename = (headers: Record<string, string>) =>
      fetch(`${AT}/views/v1`, {
        method: "PATCH",
        headers,
        body: JSON.stringify({ name: "Berlin leads" }),
      });

    const unguarded = await rename({});
    const stale = await rename({ "If-Match": "0" });
    expect([unguarded.status, stale.status]).toEqual([409, 409]);
    expect(await unguarded.json()).toMatchObject({ code: "version_skew" });
    expect(await (await fetch(`${AT}/views/v1`)).json()).toMatchObject({
      name: "Berliners",
      version: 1,
    });

    expect((await rename({ "If-Match": "1" })).status).toBe(200);
    expect(await (await fetch(`${AT}/views/v1`)).json()).toMatchObject({
      name: "Berlin leads",
      version: 2,
    });
  });
});
