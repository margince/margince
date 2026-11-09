// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { jsonResponse } from "../testing/story-utils";
import { searchByEntity } from "./relationshipcandidates";

afterEach(() => vi.unstubAllGlobals());

const rows = {
  o1: [{ id: "d1", name: "Employer renewal", company_id: "o1" }],
  o2: [{ id: "d2", name: "Second employer expansion", company_id: "o2" }],
  workspace: [
    { id: "d3", name: "Partner renewal", company_id: "o3" },
    { id: "d1", name: "Employer renewal", company_id: "o1" },
  ],
};

function stubDeals() {
  const requests: URL[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const url = new URL(
      input instanceof Request ? input.url : String(input),
      "https://test.local",
    );
    requests.push(url);
    const company = url.searchParams.get("company_id");
    const data =
      company === "o1" ? rows.o1 : company === "o2" ? rows.o2 : rows.workspace;
    return jsonResponse({ data, page: { has_more: false } });
  });
  return requests;
}

it("lists every current employer's open deals without asking for unrelated deals", async () => {
  const requests = stubDeals();
  const candidates = await searchByEntity("deal", "", ["o1", "o2", "o1"]);
  expect(requests).toHaveLength(2);
  expect(
    requests.map((url) => url.searchParams.get("company_id")).sort(),
  ).toEqual(["o1", "o2"]);
  expect(
    requests.every((url) => url.searchParams.get("status") === "open"),
  ).toBe(true);
  expect(candidates.map((candidate) => candidate.id)).toEqual(["d1", "d2"]);
});

it("searches on the server and places employer deals before other open deals without duplicates", async () => {
  const requests = stubDeals();
  const candidates = await searchByEntity("deal", "renewal", ["o1", "o2"]);
  expect(requests).toHaveLength(3);
  for (const request of requests) {
    expect(request.searchParams.get("q")).toBe("renewal");
    expect(request.searchParams.get("status")).toBe("open");
    expect(request.searchParams.get("limit")).not.toBe("50");
  }
  expect(candidates.map((candidate) => candidate.id)).toEqual([
    "d1",
    "d2",
    "d3",
  ]);
  expect(candidates[2]).toMatchObject({ companyId: "o3" });
});

it("uses the open-deal server search when no employer is known", async () => {
  const requests = stubDeals();
  await searchByEntity("deal", "older deal");
  expect(requests).toHaveLength(1);
  expect(requests[0]?.searchParams.get("q")).toBe("older deal");
  expect(requests[0]?.searchParams.get("status")).toBe("open");
  expect(requests[0]?.searchParams.has("company_id")).toBe(false);
});
